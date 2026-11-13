package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"aarvix-nexus/internal/api"
	"aarvix-nexus/internal/auth"
	"aarvix-nexus/internal/config"
	"aarvix-nexus/internal/db"
	"aarvix-nexus/internal/mailer"
	"aarvix-nexus/internal/metrics"
	"aarvix-nexus/internal/queue"
	"aarvix-nexus/internal/recovery"
	"aarvix-nexus/internal/scheduler"
)

func main() {
	cfg := config.Load()
	if len(cfg.JWTSecret) < 32 {
		log.Fatal("JWT_SECRET must be set and at least 32 characters")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	bootstrapAdmin(ctx, pool, cfg)

	pub := queue.NewPublisher(cfg.AmqpURL)
	go scheduler.New(pool, pub, cfg).Run(ctx)
	go recovery.New(pool, cfg).Run(ctx)
	go collectGauges(ctx, pool, cfg)

	go func() { // Prometheus endpoint on its own (internal) port, not on the public API
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		log.Println("metrics on", cfg.MetricsAddr)
		_ = http.ListenAndServe(cfg.MetricsAddr, mux)
	}()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(pool, cfg, mailer.New(cfg)).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Println("API listening on", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func bootstrapAdmin(ctx context.Context, pool *pgxpool.Pool, cfg config.Config) {
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		log.Println("ADMIN_EMAIL/ADMIN_PASSWORD not set: no admin bootstrapped")
		return
	}
	if err := auth.ValidPassword(cfg.AdminPassword); err != nil {
		log.Fatalf("ADMIN_PASSWORD rejected: %v", err)
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		log.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO users(id, email, password_hash, role, verified) VALUES ($1,$2,$3,'admin',true)
		ON CONFLICT (email) DO NOTHING`, uuid.NewString(), cfg.AdminEmail, hash)
	if err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}
}

func collectGauges(ctx context.Context, pool *pgxpool.Pool, cfg config.Config) {
	statuses := []string{"pending", "assigned", "running", "succeeded", "retrying", "dead", "cancelled"}
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		counts := map[string]float64{}
		if rows, err := pool.Query(ctx, `SELECT status, count(*) FROM jobs GROUP BY status`); err == nil {
			for rows.Next() {
				var s string
				var n int64
				if rows.Scan(&s, &n) == nil {
					counts[s] = float64(n)
				}
			}
			rows.Close()
			for _, s := range statuses {
				metrics.JobsByStatus.WithLabelValues(s).Set(counts[s])
			}
		}
		var n int
		if pool.QueryRow(ctx, `SELECT count(*) FROM workers WHERE status='online' AND last_heartbeat > now() - make_interval(secs => $1)`,
			cfg.HeartbeatTTL.Seconds()).Scan(&n) == nil {
			metrics.WorkersOnline.Set(float64(n))
		}
	}
}
