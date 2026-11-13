package recovery

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"aarvix-nexus/internal/config"
	"aarvix-nexus/internal/metrics"
)

// Supervisor is the failure detector. It runs next to the scheduler (any number of
// replicas is safe thanks to FOR UPDATE SKIP LOCKED) and does two things:
//  1. marks workers offline when their heartbeat is older than HeartbeatTTL;
//  2. recovers jobs whose lease expired or whose worker is offline, by requeueing them
//     (until max_attempts, then they are dead-lettered and visible to the admin).
// Recovered jobs get a new fencing token on re-assignment, so a "zombie" worker that
// comes back later cannot overwrite the result of the new attempt.
type Supervisor struct {
	pool *pgxpool.Pool
	cfg  config.Config
}

func New(pool *pgxpool.Pool, cfg config.Config) *Supervisor { return &Supervisor{pool, cfg} }

func (s *Supervisor) Run(ctx context.Context) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.sweep(ctx); err != nil && ctx.Err() == nil {
				log.Printf("recovery: %v", err)
			}
		}
	}
}

func (s *Supervisor) sweep(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		UPDATE workers SET status='offline'
		WHERE status='online' AND last_heartbeat < now() - make_interval(secs => $1)
		RETURNING id`, s.cfg.HeartbeatTTL.Seconds())
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			log.Printf("recovery: worker %s missed heartbeats -> OFFLINE", id)
			metrics.WorkersMarkedOffline.Inc()
			s.audit(ctx, "worker_offline", id)
		}
	}
	rows.Close()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	jr, err := tx.Query(ctx, `
		SELECT id, attempts, max_attempts FROM jobs
		WHERE status IN ('assigned','running')
		  AND (lease_expires_at < now() OR worker_id IN (SELECT id FROM workers WHERE status='offline'))
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return err
	}
	type lost struct {
		id            string
		attempts, max int
	}
	var list []lost
	for jr.Next() {
		var l lost
		if err := jr.Scan(&l.id, &l.attempts, &l.max); err != nil {
			jr.Close()
			return err
		}
		list = append(list, l)
	}
	jr.Close()
	if err := jr.Err(); err != nil {
		return err
	}

	for _, l := range list {
		status := "retrying"
		if l.attempts >= l.max {
			status = "dead"
		}
		if _, err := tx.Exec(ctx, `
			UPDATE job_attempts SET finished_at=now(), outcome='lost', error='worker failure or lease expired'
			WHERE job_id=$1 AND attempt_no=$2 AND finished_at IS NULL`, l.id, l.attempts); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE jobs SET status=$2::text, worker_id=NULL, lease_expires_at=NULL, run_at=now(),
			       error='recovered after worker failure or lease expiry',
			       finished_at = CASE WHEN $2::text='dead' THEN now() ELSE NULL END
			WHERE id=$1`, l.id, status); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_log(actor, action, detail) VALUES ('system','job_recovered',$1)`,
			l.id+" -> "+status); err != nil {
			return err
		}
		metrics.Recoveries.Inc()
		log.Printf("recovery: job %s interrupted (attempt %d/%d) -> %s", l.id, l.attempts, l.max, status)
	}
	return tx.Commit(ctx)
}

func (s *Supervisor) audit(ctx context.Context, action, detail string) {
	_, _ = s.pool.Exec(ctx, `INSERT INTO audit_log(actor, action, detail) VALUES ('system',$1,$2)`, action, detail)
}
