package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aarvix-nexus/internal/jobs"
	"aarvix-nexus/internal/metrics"
)

type Job struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	Priority    int             `json:"priority"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	WorkerID    *string         `json:"worker_id"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       *string         `json:"error"`
	RunAt       time.Time       `json:"run_at"`
	CreatedAt   time.Time       `json:"created_at"`
	StartedAt   *time.Time      `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at"`
}

const jobCols = `id, type, payload, status, priority, attempts, max_attempts, worker_id, result, error,
	run_at, created_at, started_at, finished_at`

type scanner interface{ Scan(dest ...any) error }

func scanJob(s scanner) (Job, error) {
	var j Job
	var payload, result []byte
	err := s.Scan(&j.ID, &j.Type, &payload, &j.Status, &j.Priority, &j.Attempts, &j.MaxAttempts,
		&j.WorkerID, &result, &j.Error, &j.RunAt, &j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	j.Payload, j.Result = payload, result
	return j, err
}

func (a *API) createJob(w http.ResponseWriter, r *http.Request) {
	c := claimsOf(r)
	var in struct {
		Type           string          `json:"type"`
		Payload        json.RawMessage `json:"payload"`
		Priority       int             `json:"priority"`
		MaxAttempts    int             `json:"max_attempts"`
		RunAt          *time.Time      `json:"run_at"`
		IdempotencyKey string          `json:"idempotency_key"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !jobs.Known[in.Type] {
		errJSON(w, 400, "unknown job type")
		return
	}
	if jobs.AdminOnly[in.Type] && c.Role != "admin" {
		errJSON(w, 403, "this job type is restricted to admins")
		return
	}
	if len(in.Payload) == 0 {
		in.Payload = json.RawMessage(`{}`)
	}
	if len(in.Payload) > 64*1024 {
		errJSON(w, 400, "payload too large (max 64KB)")
		return
	}
	var v any
	if err := json.Unmarshal(in.Payload, &v); err != nil {
		errJSON(w, 400, "payload must be valid JSON")
		return
	}
	if in.Priority == 0 {
		in.Priority = 5
	}
	if in.MaxAttempts == 0 {
		in.MaxAttempts = 3
	}
	if in.Priority < 1 || in.Priority > 10 || in.MaxAttempts < 1 || in.MaxAttempts > 10 || len(in.IdempotencyKey) > 128 {
		errJSON(w, 400, "priority 1-10, max_attempts 1-10, idempotency_key <=128 chars")
		return
	}
	runAt := time.Now()
	if in.RunAt != nil && in.RunAt.After(runAt) {
		runAt = *in.RunAt
	}

	// Canonical form (Go sorts map keys) -> stable hash for duplicate detection.
	canon, _ := json.Marshal(v)
	sum := sha256.Sum256([]byte(c.Subject + "|" + in.Type + "|" + string(canon)))
	hash := hex.EncodeToString(sum[:])

	ctx := r.Context()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	defer tx.Rollback(ctx)

	// Serialise concurrent identical submissions so both cannot pass the duplicate check.
	lockKey := hash
	if in.IdempotencyKey != "" {
		lockKey = c.Subject + "|" + in.IdempotencyKey
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
		errJSON(w, 500, "internal error")
		return
	}

	var existingID, existingStatus string
	if in.IdempotencyKey != "" {
		err = tx.QueryRow(ctx, `SELECT id, status FROM jobs WHERE created_by=$1 AND idempotency_key=$2`,
			c.Subject, in.IdempotencyKey).Scan(&existingID, &existingStatus)
	} else {
		err = tx.QueryRow(ctx, `
			SELECT id, status FROM jobs
			WHERE payload_hash=$1 AND status NOT IN ('cancelled','dead')
			  AND created_at > now() - make_interval(secs => $2)
			ORDER BY created_at DESC LIMIT 1`, hash, a.cfg.DupWindow.Seconds()).Scan(&existingID, &existingStatus)
	}
	if err == nil {
		metrics.DuplicatesBlocked.Inc()
		a.audit(ctx, c.Email, "duplicate_blocked", existingID)
		writeJSON(w, 200, map[string]any{"job_id": existingID, "status": existingStatus, "duplicate": true})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		errJSON(w, 500, "internal error")
		return
	}

	id := uuid.NewString()
	var key *string
	if in.IdempotencyKey != "" {
		key = &in.IdempotencyKey
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO jobs(id, type, payload, priority, run_at, idempotency_key, payload_hash, max_attempts, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, in.Type, []byte(in.Payload), in.Priority, runAt, key, hash, in.MaxAttempts, c.Subject)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	writeJSON(w, 201, map[string]any{"job_id": id, "status": "pending", "duplicate": false})
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	c := claimsOf(r)
	status := r.URL.Query().Get("status")
	switch status {
	case "", "pending", "assigned", "running", "succeeded", "retrying", "dead", "cancelled":
	default:
		errJSON(w, 400, "invalid status filter")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := a.pool.Query(r.Context(), `SELECT `+jobCols+` FROM jobs
		WHERE ($1::boolean OR created_by = $2) AND ($3 = '' OR status = $3)
		ORDER BY created_at DESC LIMIT $4`, c.Role == "admin", c.Subject, status, limit)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			errJSON(w, 500, "internal error")
			return
		}
		out = append(out, j)
	}
	writeJSON(w, 200, out)
}

func (a *API) getJob(w http.ResponseWriter, r *http.Request) {
	c := claimsOf(r)
	j, err := scanJob(a.pool.QueryRow(r.Context(), `SELECT `+jobCols+` FROM jobs
		WHERE id=$1 AND ($2::boolean OR created_by=$3)`, r.PathValue("id"), c.Role == "admin", c.Subject))
	if err != nil {
		errJSON(w, 404, "job not found")
		return
	}
	type attempt struct {
		No         int        `json:"attempt_no"`
		WorkerID   string     `json:"worker_id"`
		StartedAt  time.Time  `json:"started_at"`
		FinishedAt *time.Time `json:"finished_at"`
		Outcome    *string    `json:"outcome"`
		Error      *string    `json:"error"`
	}
	history := []attempt{}
	rows, err := a.pool.Query(r.Context(), `SELECT attempt_no, worker_id, started_at, finished_at, outcome, error
		FROM job_attempts WHERE job_id=$1 ORDER BY attempt_no`, j.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var at attempt
			if rows.Scan(&at.No, &at.WorkerID, &at.StartedAt, &at.FinishedAt, &at.Outcome, &at.Error) == nil {
				history = append(history, at)
			}
		}
	}
	writeJSON(w, 200, map[string]any{"job": j, "attempts": history})
}

func (a *API) cancelJob(w http.ResponseWriter, r *http.Request) {
	c := claimsOf(r)
	tag, err := a.pool.Exec(r.Context(), `
		UPDATE jobs SET status='cancelled', finished_at=now(), lease_expires_at=NULL
		WHERE id=$1 AND ($2::boolean OR created_by=$3) AND status IN ('pending','retrying','assigned','running')`,
		r.PathValue("id"), c.Role == "admin", c.Subject)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, 404, "job not found or not cancellable")
		return
	}
	a.audit(r.Context(), c.Email, "job_cancelled", r.PathValue("id"))
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}
