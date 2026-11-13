# AARVIX NEXUS – Self-Healing Distributed Cloud Platform
DATAMINT AARVIX · internal product project

Go · PostgreSQL · RabbitMQ · Docker · Prometheus · Grafana · Android (Kotlin / Jetpack Compose)

> **Status: first implementation, not yet compiled or run by its author.** It was written without a Go or
> Android toolchain available. Expect to fix a few compile errors on the first `docker compose build`
> and Android Studio sync. Treat the test/chaos results as something you still have to produce and document.

## Run it
```bash
cp .env.example .env        # then CHANGE every secret
docker compose up --build
```
- API: http://localhost:8080 · Grafana: http://localhost:3000 (admin / GRAFANA_PASSWORD) · RabbitMQ UI: http://localhost:15672 · Prometheus: http://localhost:9090
- The admin account is created from `ADMIN_EMAIL` / `ADMIN_PASSWORD` on first start.
- With `SMTP_HOST` empty, verification codes are printed in `docker compose logs server` (dev only).
- Go unit tests: `go test ./...` · Failure test: `ADMIN_EMAIL=... ADMIN_PASSWORD=... ./scripts/chaos_test.sh`

## Android app (`android/`)
Open the `android` folder in Android Studio (it generates the Gradle wrapper), run on an emulator
(`BASE_URL` points to `10.0.2.2:8080`). For a phone, change `BASE_URL` in `app/build.gradle.kts` to your
PC's LAN IP and allow that host in `network_security_config.xml`; production must use HTTPS.
Design: indigo→violet→cyan brand gradient, light and dark themes (follows the phone), rounded cards, status chips,
icons and live progress bars (`Theme.kt`, `Components.kt`, `Screens.kt`).
Screens: Login · Register (live password rules) · Email verification (6-digit code, resend) ·
My jobs (summary tiles, filters, live status, cancel) · New job (type descriptions, priority) ·
**Admin panel** (admin role only): 12 colour-coded stat cards, worker health with job/CPU meters,
failed/retrying jobs with Retry, users.

## How the requirements are met
| Requirement | Implementation |
|---|---|
| **Smart worker selection** | `scheduler/select.go`: score = 45% free capacity + 30% historical success rate − 15% host load − 10% latency. Jobs are routed to one specific worker queue. A retried job avoids workers that already failed/lost it. |
| **Fault detection** | Workers heartbeat every 5s (only while connected to RabbitMQ). `recovery/` marks a worker offline after 15s of silence. Job leases (30s, renewed while running) detect hung/crashed executions. |
| **Failure recovery** | Interrupted jobs (expired lease or offline worker) go back to `retrying` immediately; failed jobs retry with exponential backoff + jitter up to `max_attempts`, then become `dead` (admin can Retry). Permanent errors skip retries. |
| **Duplicate detection** | (1) `idempotency_key` per user (unique index); (2) otherwise SHA-256 of user+type+canonical payload within `DUP_WINDOW` (60s), serialised with an advisory lock so concurrent submits can't both pass. Duplicates return the existing job (`duplicate:true`). |
| **No duplicate business operations** | At-least-once delivery + idempotent effects: `business_ops.op_key` is a primary key (`ON CONFLICT DO NOTHING`). **Fencing tokens** stop a zombie worker from overwriting a reassigned job's result. |
| **Security** | bcrypt(12), password policy, JWT HS256 (30 min) with role claim, email verification (hashed, expiring, max 5 guesses), account lockout after 5 failures, per-IP rate limits, no user enumeration, admin-only routes, chaos job types admin-only, 1MB body limit, security headers, metrics on an internal port, secrets via env, Android token in EncryptedSharedPreferences, HTTPS-only network config except emulator host. |
| **Performance** | `FOR UPDATE SKIP LOCKED` batch dispatch, RabbitMQ prefetch = worker capacity, indexed queues, horizontally scalable server replicas and workers, Prometheus latency histogram for benchmarking. |

## Job lifecycle
`pending → assigned → running → succeeded`  ·  failure: `→ retrying → … → dead`  ·  `cancelled`
Assignment (owner, lease, fence) is committed to PostgreSQL **before** the message is published, so a crash
or broker failure at any point is recovered by lease expiry.

## API (JSON, `Authorization: Bearer <token>`)
`POST /api/auth/register|verify|resend|login` · `GET /api/me` ·
`POST /api/jobs` `{type,payload,priority,max_attempts,idempotency_key,run_at}` · `GET /api/jobs?status=` ·
`GET /api/jobs/{id}` (with attempt history) · `POST /api/jobs/{id}/cancel` ·
admin: `GET /api/admin/stats|workers|users|audit` · `POST /api/admin/jobs/{id}/retry`

Job types: `echo`, `sleep {seconds}`, `compute {n}`, `business_op {account, amount_cents, op_key?}`;
admin-only test types: `flaky {fail_times}`, `fail`, `crash {on_attempt}` (needs `ENABLE_CHAOS_JOBS=true`).

## Known limitations (be honest in the architecture review)
- Single PostgreSQL and RabbitMQ node: no HA for the infrastructure itself.
- JWT roles are valid until expiry (30 min); there is no refresh token or revocation list.
- Rate limiter is in-memory per server replica; behind a proxy, use the proxy's real client IP.
- Workers connect to PostgreSQL directly (simple, fast); a larger deployment should add a worker API.
- No tests against a real database yet: add integration tests (testcontainers) for claim/fence/recovery paths.
