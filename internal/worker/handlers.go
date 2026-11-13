package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrPermanent marks failures that retrying cannot fix (bad input etc.): job goes straight to "dead".
var ErrPermanent = errors.New("permanent failure")

func perm(msg string) error { return fmt.Errorf("%w: %s", ErrPermanent, msg) }

type Env struct {
	JobID   string
	Type    string
	Attempt int
	Payload json.RawMessage
	Pool    *pgxpool.Pool
	Chaos   bool
}

// Execute runs one job. Handlers must honour ctx (timeout / lost lease) and be idempotent,
// because the platform guarantees at-least-once execution.
func Execute(ctx context.Context, e Env) (any, error) {
	switch e.Type {
	case "echo":
		var v any
		_ = json.Unmarshal(e.Payload, &v)
		return map[string]any{"echo": v}, nil

	case "sleep":
		var p struct {
			Seconds float64 `json:"seconds"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil || p.Seconds < 0 || p.Seconds > 300 {
			return nil, perm(`payload must be {"seconds": 0..300}`)
		}
		select {
		case <-time.After(time.Duration(p.Seconds * float64(time.Second))):
			return map[string]any{"slept_seconds": p.Seconds}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}

	case "compute": // counts primes below n with a sieve
		var p struct {
			N int `json:"n"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil || p.N < 2 || p.N > 5_000_000 {
			return nil, perm(`payload must be {"n": 2..5000000}`)
		}
		comp := make([]bool, p.N+1)
		count := 0
		for i := 2; i <= p.N; i++ {
			if i%100000 == 0 && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !comp[i] {
				count++
				for j := i * i; j <= p.N; j += i {
					comp[j] = true
				}
			}
		}
		return map[string]any{"n": p.N, "primes": count}, nil

	case "business_op":
		// Exactly-once EFFECT on top of at-least-once delivery: op_key is a primary key.
		var p struct {
			OpKey       string `json:"op_key"`
			Account     string `json:"account"`
			AmountCents int64  `json:"amount_cents"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil || p.Account == "" || p.AmountCents <= 0 {
			return nil, perm(`payload must be {"account": "...", "amount_cents": >0, "op_key": optional}`)
		}
		if p.OpKey == "" {
			p.OpKey = e.JobID
		}
		tag, err := e.Pool.Exec(ctx, `
			INSERT INTO business_ops(op_key, job_id, account, amount_cents) VALUES ($1,$2,$3,$4)
			ON CONFLICT (op_key) DO NOTHING`, p.OpKey, e.JobID, p.Account, p.AmountCents)
		if err != nil {
			return nil, err // transient DB error -> retry
		}
		if tag.RowsAffected() == 0 {
			return map[string]any{"applied": false, "reason": "duplicate operation suppressed", "op_key": p.OpKey}, nil
		}
		return map[string]any{"applied": true, "op_key": p.OpKey}, nil

	case "flaky": // fails the first N attempts, then succeeds: demonstrates retry
		var p struct {
			FailTimes int `json:"fail_times"`
		}
		p.FailTimes = 1
		_ = json.Unmarshal(e.Payload, &p)
		if e.Attempt <= p.FailTimes {
			return nil, fmt.Errorf("simulated transient failure on attempt %d", e.Attempt)
		}
		return map[string]any{"recovered_on_attempt": e.Attempt}, nil

	case "fail":
		return nil, perm("simulated permanent failure")

	case "crash": // kills the worker process: demonstrates failure detection + reassignment
		if !e.Chaos {
			return nil, perm("chaos jobs are disabled (ENABLE_CHAOS_JOBS=true)")
		}
		var p struct {
			OnAttempt int `json:"on_attempt"`
		}
		p.OnAttempt = 1
		_ = json.Unmarshal(e.Payload, &p)
		if e.Attempt <= p.OnAttempt {
			log.Printf("chaos: crashing worker on attempt %d of job %s", e.Attempt, e.JobID)
			os.Exit(1)
		}
		return map[string]any{"survived_attempt": e.Attempt}, nil
	}
	return nil, perm("unknown job type " + e.Type)
}
