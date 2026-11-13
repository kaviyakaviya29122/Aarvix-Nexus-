package api

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"aarvix-nexus/internal/auth"
	"aarvix-nexus/internal/config"
	"aarvix-nexus/internal/mailer"
)

type API struct {
	pool   *pgxpool.Pool
	cfg    config.Config
	mail   mailer.Mailer
	secret []byte
	authRL *limiter // strict: register/login/verify
	genRL  *limiter // general per-IP limit
}

func New(pool *pgxpool.Pool, cfg config.Config, m mailer.Mailer) *API {
	return &API{pool: pool, cfg: cfg, mail: m, secret: []byte(cfg.JWTSecret),
		authRL: newLimiter(10, time.Minute), genRL: newLimiter(300, time.Minute)}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.pool.Ping(ctx); err != nil {
			errJSON(w, 503, "database unavailable")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})

	// Authentication + email verification
	mux.HandleFunc("POST /api/auth/register", a.limit(a.authRL, a.register))
	mux.HandleFunc("POST /api/auth/verify", a.limit(a.authRL, a.verify))
	mux.HandleFunc("POST /api/auth/resend", a.limit(a.authRL, a.resend))
	mux.HandleFunc("POST /api/auth/login", a.limit(a.authRL, a.login))
	mux.HandleFunc("GET /api/me", a.authed(false, a.me))

	// Jobs (owner or admin)
	mux.HandleFunc("POST /api/jobs", a.authed(false, a.createJob))
	mux.HandleFunc("GET /api/jobs", a.authed(false, a.listJobs))
	mux.HandleFunc("GET /api/jobs/{id}", a.authed(false, a.getJob))
	mux.HandleFunc("POST /api/jobs/{id}/cancel", a.authed(false, a.cancelJob))

	// Admin panel
	mux.HandleFunc("GET /api/admin/stats", a.authed(true, a.adminStats))
	mux.HandleFunc("GET /api/admin/workers", a.authed(true, a.adminWorkers))
	mux.HandleFunc("GET /api/admin/users", a.authed(true, a.adminUsers))
	mux.HandleFunc("GET /api/admin/audit", a.authed(true, a.adminAudit))
	mux.HandleFunc("POST /api/admin/jobs/{id}/retry", a.authed(true, a.adminRetry))

	return secure(a.limit(a.genRL, mux.ServeHTTP))
}

// ---------- middleware ----------

func secure(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'")
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %s", r.Method, r.URL.Path, clientIP(r), time.Since(start).Round(time.Millisecond))
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *API) limit(l *limiter, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			errJSON(w, 429, "too many requests")
			return
		}
		h(w, r)
	}
}

type ctxKey struct{}

func claimsOf(r *http.Request) *auth.Claims { return r.Context().Value(ctxKey{}).(*auth.Claims) }

func (a *API) authed(adminOnly bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Authorization")
		tok := strings.TrimPrefix(hdr, "Bearer ")
		if tok == "" || tok == hdr {
			errJSON(w, 401, "missing bearer token")
			return
		}
		c, err := auth.Parse(a.secret, tok)
		if err != nil {
			errJSON(w, 401, "invalid or expired token")
			return
		}
		if adminOnly && c.Role != "admin" {
			errJSON(w, 403, "admin only")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, c)))
	}
}

// ---------- rate limiter (fixed window per key) ----------

type win struct {
	start time.Time
	n     int
}

type limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	m      map[string]*win
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{limit: limit, window: window, m: map[string]*win{}}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	w, ok := l.m[key]
	if !ok || now.Sub(w.start) > l.window {
		if len(l.m) > 10000 {
			for k, v := range l.m {
				if now.Sub(v.start) > l.window {
					delete(l.m, k)
				}
			}
		}
		l.m[key] = &win{start: now, n: 1}
		return true
	}
	w.n++
	return w.n <= l.limit
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		errJSON(w, 400, "invalid JSON body")
		return false
	}
	return true
}

func (a *API) audit(ctx context.Context, actor, action, detail string) {
	_, _ = a.pool.Exec(ctx, `INSERT INTO audit_log(actor, action, detail) VALUES ($1,$2,$3)`, actor, action, detail)
}
