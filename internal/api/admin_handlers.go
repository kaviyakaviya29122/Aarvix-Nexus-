package api

import (
	"net/http"
	"strconv"
	"time"
)

func (a *API) adminStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	byStatus := map[string]int64{}
	rows, err := a.pool.Query(ctx, `SELECT status, count(*) FROM jobs GROUP BY status`)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	for rows.Next() {
		var s string
		var n int64
		if rows.Scan(&s, &n) == nil {
			byStatus[s] = n
		}
	}
	rows.Close()

	var online, offline int
	var avgMS float64
	var lost, failed, dups, perMin int64
	_ = a.pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE status='online' AND last_heartbeat > now() - make_interval(secs => $1)),
		count(*) FILTER (WHERE NOT (status='online' AND last_heartbeat > now() - make_interval(secs => $1)))
		FROM workers`, a.cfg.HeartbeatTTL.Seconds()).Scan(&online, &offline)
	_ = a.pool.QueryRow(ctx, `SELECT COALESCE(avg(extract(epoch FROM (finished_at - started_at))*1000),0)::float8
		FROM jobs WHERE status='succeeded' AND finished_at > now() - interval '1 hour'`).Scan(&avgMS)
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM job_attempts WHERE outcome='lost'`).Scan(&lost)
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM job_attempts WHERE outcome='failed'`).Scan(&failed)
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action='duplicate_blocked'`).Scan(&dups)
	_ = a.pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE status='succeeded' AND finished_at > now() - interval '1 minute'`).Scan(&perMin)

	writeJSON(w, 200, map[string]any{
		"jobs_by_status":     byStatus,
		"workers_online":     online,
		"workers_offline":    offline,
		"avg_ms":             avgMS,
		"recovered_attempts": lost,
		"failed_attempts":    failed,
		"duplicates_blocked": dups,
		"throughput_per_min": perMin,
	})
}

func (a *API) adminWorkers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pool.Query(r.Context(), `
		SELECT w.id,
		       CASE WHEN w.status='online' AND w.last_heartbeat > now() - make_interval(secs => $1) THEN 'online' ELSE 'offline' END,
		       w.capacity, w.load, w.last_heartbeat, w.success_count, w.fail_count, w.avg_ms,
		       (SELECT count(*) FROM jobs j WHERE j.worker_id=w.id AND j.status IN ('assigned','running'))
		FROM workers w ORDER BY w.id`, a.cfg.HeartbeatTTL.Seconds())
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	defer rows.Close()
	type wk struct {
		ID            string    `json:"id"`
		Status        string    `json:"status"`
		Capacity      int       `json:"capacity"`
		Load          float64   `json:"load"`
		LastHeartbeat time.Time `json:"last_heartbeat"`
		SuccessCount  int64     `json:"success_count"`
		FailCount     int64     `json:"fail_count"`
		AvgMS         float64   `json:"avg_ms"`
		ActiveJobs    int64     `json:"active_jobs"`
	}
	out := []wk{}
	for rows.Next() {
		var x wk
		if rows.Scan(&x.ID, &x.Status, &x.Capacity, &x.Load, &x.LastHeartbeat, &x.SuccessCount, &x.FailCount, &x.AvgMS, &x.ActiveJobs) == nil {
			out = append(out, x)
		}
	}
	writeJSON(w, 200, out)
}

func (a *API) adminUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pool.Query(r.Context(), `SELECT id, email, role, verified, created_at FROM users ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	defer rows.Close()
	type u struct {
		ID        string    `json:"id"`
		Email     string    `json:"email"`
		Role      string    `json:"role"`
		Verified  bool      `json:"verified"`
		CreatedAt time.Time `json:"created_at"`
	}
	out := []u{}
	for rows.Next() {
		var x u
		if rows.Scan(&x.ID, &x.Email, &x.Role, &x.Verified, &x.CreatedAt) == nil {
			out = append(out, x)
		}
	}
	writeJSON(w, 200, out)
}

func (a *API) adminAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := a.pool.Query(r.Context(), `SELECT actor, action, COALESCE(detail,''), created_at FROM audit_log ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	defer rows.Close()
	type e struct {
		Actor     string    `json:"actor"`
		Action    string    `json:"action"`
		Detail    string    `json:"detail"`
		CreatedAt time.Time `json:"created_at"`
	}
	out := []e{}
	for rows.Next() {
		var x e
		if rows.Scan(&x.Actor, &x.Action, &x.Detail, &x.CreatedAt) == nil {
			out = append(out, x)
		}
	}
	writeJSON(w, 200, out)
}

// adminRetry re-queues a dead or cancelled job with a fresh attempt budget.
func (a *API) adminRetry(w http.ResponseWriter, r *http.Request) {
	tag, err := a.pool.Exec(r.Context(), `
		UPDATE jobs SET status='pending', attempts=0, run_at=now(), error=NULL, finished_at=NULL,
		       worker_id=NULL, lease_expires_at=NULL
		WHERE id=$1 AND status IN ('dead','cancelled')`, r.PathValue("id"))
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, 404, "job not found or not in a retryable state")
		return
	}
	a.audit(r.Context(), claimsOf(r).Email, "job_retried", r.PathValue("id"))
	writeJSON(w, 200, map[string]string{"status": "pending"})
}
