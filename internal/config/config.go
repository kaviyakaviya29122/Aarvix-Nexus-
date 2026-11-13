package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr    string
	MetricsAddr string
	DatabaseURL string
	AmqpURL     string
	JWTSecret   string

	AdminEmail    string
	AdminPassword string

	SMTPHost, SMTPPort, SMTPUser, SMTPPass, SMTPFrom string

	LeaseTTL     time.Duration // how long a worker owns a job without renewing
	HeartbeatTTL time.Duration // worker considered dead after this silence
	JobTimeout   time.Duration
	DupWindow    time.Duration // identical payloads inside this window are duplicates

	WorkerID       string
	WorkerCapacity int
	EnableChaos    bool // allows the "crash" job type (testing only)
}

func Load() Config {
	host, _ := os.Hostname()
	return Config{
		HTTPAddr:       get("HTTP_ADDR", ":8080"),
		MetricsAddr:    get("METRICS_ADDR", ":9100"),
		DatabaseURL:    get("DATABASE_URL", "postgres://nexus:nexus@localhost:5432/nexus?sslmode=disable"),
		AmqpURL:        get("AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		AdminEmail:     os.Getenv("ADMIN_EMAIL"),
		AdminPassword:  os.Getenv("ADMIN_PASSWORD"),
		SMTPHost:       os.Getenv("SMTP_HOST"),
		SMTPPort:       get("SMTP_PORT", "587"),
		SMTPUser:       os.Getenv("SMTP_USER"),
		SMTPPass:       os.Getenv("SMTP_PASS"),
		SMTPFrom:       get("SMTP_FROM", "no-reply@aarvix.local"),
		LeaseTTL:       dur("LEASE_TTL", 30*time.Second),
		HeartbeatTTL:   dur("HEARTBEAT_TTL", 15*time.Second),
		JobTimeout:     dur("JOB_TIMEOUT", 5*time.Minute),
		DupWindow:      dur("DUP_WINDOW", 60*time.Second),
		WorkerID:       get("WORKER_ID", host),
		WorkerCapacity: num("WORKER_CAPACITY", 4),
		EnableChaos:    os.Getenv("ENABLE_CHAOS_JOBS") == "true",
	}
}

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func num(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}

func dur(k string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}
