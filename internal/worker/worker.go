package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"aarvix-nexus/internal/config"
	"aarvix-nexus/internal/jobs"
	"aarvix-nexus/internal/metrics"
	"aarvix-nexus/internal/queue"
)

type Worker struct {
	cfg       config.Config
	pool      *pgxpool.Pool
	id        string
	connected atomic.Bool
}

func New(cfg config.Config, pool *pgxpool.Pool) *Worker {
	return &Worker{cfg: cfg, pool: pool, id: cfg.WorkerID}
}

func (w *Worker) Run(ctx context.Context) {
	go w.heartbeatLoop(ctx)
	for ctx.Err() == nil {
		if err := w.session(ctx); err != nil {
			log.Printf("worker %s: session ended: %v", w.id, err)
		}
		w.connected.Store(false)
		select {
		case <-ctx.Done():
		case <-time.After(3 * time.Second):
		}
	}
	// Graceful shutdown: tell the platform immediately so in-flight jobs are recovered fast.
	_, _ = w.pool.Exec(context.Background(), `UPDATE workers SET status='offline' WHERE id=$1`, w.id)
}

func (w *Worker) session(ctx context.Context) error {
	conn, err := queue.Dial(w.cfg.AmqpURL)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	if err := queue.Declare(ch); err != nil {
		return err
	}
	q, err := queue.DeclareWorkerQueue(ch, w.id)
	if err != nil {
		return err
	}
	if err := ch.Qos(w.cfg.WorkerCapacity, 0, false); err != nil { // back-pressure
		return err
	}
	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	closed := conn.NotifyClose(make(chan *amqp.Error, 1))
	w.connected.Store(true)
	w.beat(ctx) // register immediately
	log.Printf("worker %s: consuming %s (capacity %d)", w.id, q.Name, w.cfg.WorkerCapacity)

	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-closed:
			return fmt.Errorf("connection closed: %v", e)
		case d, ok := <-msgs:
			if !ok {
				return errors.New("delivery channel closed")
			}
			go w.handle(ctx, d)
		}
	}
}

func (w *Worker) heartbeatLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.beat(ctx)
		}
	}
}

// beat only runs while the broker connection is healthy: a worker that cannot receive
// work must not look alive to the scheduler.
func (w *Worker) beat(ctx context.Context) {
	if !w.connected.Load() {
		return
	}
	_, err := w.pool.Exec(ctx, `
		INSERT INTO workers(id, capacity, load, status, last_heartbeat) VALUES ($1,$2,$3,'online',now())
		ON CONFLICT (id) DO UPDATE SET capacity=EXCLUDED.capacity, load=EXCLUDED.load,
		       status='online', last_heartbeat=now()`, w.id, w.cfg.WorkerCapacity, readLoad())
	if err != nil && ctx.Err() == nil {
		log.Printf("worker %s: heartbeat failed: %v", w.id, err)
	}
}

func (w *Worker) handle(ctx context.Context, d amqp.Delivery) {
	var m jobs.Message
	if err := json.Unmarshal(d.Body, &m); err != nil {
		log.Printf("worker %s: dropping malformed message: %v", w.id, err)
		_ = d.Reject(false)
		return
	}

	// CLAIM: only succeeds if this worker still owns the job at this fencing token.
	tag, err := w.pool.Exec(ctx, `
		UPDATE jobs SET status='running', started_at=COALESCE(started_at, now()),
		       lease_expires_at = now() + make_interval(secs => $4)
		WHERE id=$1 AND fence=$2 AND worker_id=$3 AND status='assigned'`,
		m.JobID, m.Fence, w.id, w.cfg.LeaseTTL.Seconds())
	if err != nil {
		log.Printf("worker %s: claim failed: %v", w.id, err)
		time.Sleep(time.Second)
		_ = d.Nack(false, true)
		return
	}
	if tag.RowsAffected() == 0 { // stale, duplicate or cancelled delivery
		metrics.StaleDeliveries.Inc()
		_ = d.Ack(false)
		return
	}

	jctx, cancel := context.WithTimeout(ctx, w.cfg.JobTimeout)
	defer cancel()
	var lost atomic.Bool
	go w.keepLease(jctx, cancel, m, &lost)

	start := time.Now()
	res, execErr := Execute(jctx, Env{JobID: m.JobID, Type: m.Type, Attempt: m.Attempt,
		Payload: m.Payload, Pool: w.pool, Chaos: w.cfg.EnableChaos})
	dur := time.Since(start)

	if lost.Load() || ctx.Err() != nil {
		// Ownership lost (cancelled / reassigned) or shutting down: write nothing,
		// the platform already knows (or the supervisor will recover it).
		log.Printf("worker %s: abandoning job %s (lost ownership or shutdown)", w.id, m.JobID)
		_ = d.Ack(false)
		return
	}
	if execErr == nil {
		w.succeed(ctx, m, res, dur)
	} else {
		w.fail(ctx, m, execErr, dur)
	}
	_ = d.Ack(false)
}

// keepLease renews the lease while the job runs. If the UPDATE matches 0 rows, someone
// else owns the job now (cancelled or recovered): abort immediately (fencing).
func (w *Worker) keepLease(ctx context.Context, cancel context.CancelFunc, m jobs.Message, lost *atomic.Bool) {
	t := time.NewTicker(w.cfg.LeaseTTL / 3)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tag, err := w.pool.Exec(ctx, `
				UPDATE jobs SET lease_expires_at = now() + make_interval(secs => $3)
				WHERE id=$1 AND fence=$2 AND status='running'`, m.JobID, m.Fence, w.cfg.LeaseTTL.Seconds())
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("worker %s: lease renewal error: %v", w.id, err)
				continue
			}
			if tag.RowsAffected() == 0 {
				lost.Store(true)
				cancel()
				return
			}
		}
	}
}

func (w *Worker) succeed(ctx context.Context, m jobs.Message, res any, dur time.Duration) {
	raw, _ := json.Marshal(res)
	ms := float64(dur.Milliseconds())
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		log.Printf("worker %s: begin: %v", w.id, err)
		return
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE jobs SET status='succeeded', result=$3, error=NULL, finished_at=now(), lease_expires_at=NULL
		WHERE id=$1 AND fence=$2 AND status='running'`, m.JobID, m.Fence, raw)
	if err != nil || tag.RowsAffected() == 0 {
		return // fenced out: another attempt owns the job now
	}
	_, _ = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now(), outcome='succeeded' WHERE job_id=$1 AND attempt_no=$2`, m.JobID, m.Attempt)
	_, _ = tx.Exec(ctx, `
		UPDATE workers SET success_count=success_count+1,
		       avg_ms = CASE WHEN success_count=0 THEN $2::float8 ELSE avg_ms*0.8 + $2::float8*0.2 END
		WHERE id=$1`, w.id, ms)
	if err := tx.Commit(ctx); err != nil {
		log.Printf("worker %s: commit success: %v (lease expiry will recover)", w.id, err)
		return
	}
	metrics.JobsFinished.WithLabelValues("succeeded").Inc()
	metrics.JobDuration.Observe(dur.Seconds())
}

func (w *Worker) fail(ctx context.Context, m jobs.Message, execErr error, dur time.Duration) {
	msg := execErr.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	permanent := errors.Is(execErr, ErrPermanent)
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `
		UPDATE jobs SET
		   status = CASE WHEN $3::boolean OR attempts >= max_attempts THEN 'dead' ELSE 'retrying' END,
		   run_at = now() + make_interval(secs => $4),
		   error = $5, worker_id = NULL, lease_expires_at = NULL,
		   finished_at = CASE WHEN $3::boolean OR attempts >= max_attempts THEN now() ELSE NULL END
		WHERE id=$1 AND fence=$2 AND status='running'
		RETURNING status`,
		m.JobID, m.Fence, permanent, jobs.Backoff(m.Attempt).Seconds(), msg).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return
	}
	_, _ = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now(), outcome='failed', error=$3 WHERE job_id=$1 AND attempt_no=$2`, m.JobID, m.Attempt, msg)
	_, _ = tx.Exec(ctx, `UPDATE workers SET fail_count=fail_count+1 WHERE id=$1`, w.id)
	if err := tx.Commit(ctx); err != nil {
		return
	}
	metrics.JobsFinished.WithLabelValues("failed").Inc()
	metrics.JobDuration.Observe(dur.Seconds())
	log.Printf("worker %s: job %s attempt %d failed -> %s (%s)", w.id, m.JobID, m.Attempt, status, msg)
}

func readLoad() float64 {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0
	}
	return math.Min(v/float64(runtime.NumCPU()), 1)
}
