package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"aarvix-nexus/internal/config"
	"aarvix-nexus/internal/jobs"
	"aarvix-nexus/internal/queue"
)

type Scheduler struct {
	pool *pgxpool.Pool
	pub  *queue.Publisher
	cfg  config.Config
}

func New(pool *pgxpool.Pool, pub *queue.Publisher, cfg config.Config) *Scheduler {
	return &Scheduler{pool: pool, pub: pub, cfg: cfg}
}

func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.dispatch(ctx); err != nil && ctx.Err() == nil {
				log.Printf("scheduler: %v", err)
			}
		}
	}
}

func (s *Scheduler) candidates(ctx context.Context) ([]Candidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT w.id, w.capacity, w.load, w.avg_ms, w.success_count, w.fail_count,
		       (SELECT count(*) FROM jobs j WHERE j.worker_id = w.id AND j.status IN ('assigned','running'))
		FROM workers w
		WHERE w.status = 'online' AND w.last_heartbeat > now() - make_interval(secs => $1)`,
		s.cfg.HeartbeatTTL.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		var c Candidate
		var active int64
		if err := rows.Scan(&c.ID, &c.Capacity, &c.Load, &c.AvgMS, &c.Success, &c.Fail, &active); err != nil {
			return nil, err
		}
		c.Active = int(active)
		out = append(out, c)
	}
	return out, rows.Err()
}

type pendingJob struct {
	id, typ  string
	payload  []byte
	attempts int
}

type outgoing struct {
	worker string
	msg    jobs.Message
}

// dispatch assigns due jobs to the best worker. Assignment (status, owner, lease, fencing
// token) is committed to PostgreSQL BEFORE publishing. If publishing fails or the worker
// dies, the lease expires and the recovery supervisor requeues the job: nothing is lost.
func (s *Scheduler) dispatch(ctx context.Context) error {
	cands, err := s.candidates(ctx)
	if err != nil || len(cands) == 0 {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, type, payload, attempts FROM jobs
		WHERE status IN ('pending','retrying') AND run_at <= now()
		ORDER BY priority DESC, run_at
		LIMIT 50 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return err
	}
	var list []pendingJob
	for rows.Next() {
		var j pendingJob
		if err := rows.Scan(&j.id, &j.typ, &j.payload, &j.attempts); err != nil {
			rows.Close()
			return err
		}
		list = append(list, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	var outs []outgoing
	for _, j := range list {
		excl := map[string]bool{}
		if j.attempts > 0 {
			er, err := tx.Query(ctx, `SELECT DISTINCT worker_id FROM job_attempts WHERE job_id=$1 AND outcome IN ('failed','lost')`, j.id)
			if err != nil {
				return err
			}
			for er.Next() {
				var w string
				if err := er.Scan(&w); err == nil {
					excl[w] = true
				}
			}
			er.Close()
		}
		i := Select(cands, excl)
		if i < 0 {
			break // every worker is full; try again next tick
		}
		var fence int64
		var attempt int
		err := tx.QueryRow(ctx, `
			UPDATE jobs SET status='assigned', worker_id=$2, fence=fence+1, attempts=attempts+1,
			       lease_expires_at = now() + make_interval(secs => $3), error=NULL
			WHERE id=$1 RETURNING fence, attempts`,
			j.id, cands[i].ID, s.cfg.LeaseTTL.Seconds()).Scan(&fence, &attempt)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO job_attempts(job_id, attempt_no, worker_id) VALUES ($1,$2,$3)`,
			j.id, attempt, cands[i].ID); err != nil {
			return err
		}
		cands[i].Active++
		outs = append(outs, outgoing{cands[i].ID, jobs.Message{
			JobID: j.id, Fence: fence, Attempt: attempt, Type: j.typ, Payload: j.payload}})
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	for _, o := range outs {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := s.pub.Publish(pctx, o.worker, o.msg); err != nil {
			log.Printf("scheduler: publish job=%s worker=%s failed (%v); lease expiry will recover it", o.msg.JobID, o.worker, err)
		}
		cancel()
	}
	return nil
}
