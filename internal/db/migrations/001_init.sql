CREATE TABLE IF NOT EXISTS users (
    id             TEXT PRIMARY KEY,
    email          TEXT UNIQUE NOT NULL,
    password_hash  TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
    verified       BOOLEAN NOT NULL DEFAULT FALSE,
    verify_hash    TEXT,
    verify_expires TIMESTAMPTZ,
    failed_logins  INT NOT NULL DEFAULT 0,
    locked_until   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS workers (
    id             TEXT PRIMARY KEY,
    status         TEXT NOT NULL DEFAULT 'online' CHECK (status IN ('online','offline')),
    capacity       INT NOT NULL DEFAULT 4,
    load           DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT now(),
    success_count  BIGINT NOT NULL DEFAULT 0,
    fail_count     BIGINT NOT NULL DEFAULT 0,
    avg_ms         DOUBLE PRECISION NOT NULL DEFAULT 0,
    started_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS jobs (
    id               TEXT PRIMARY KEY,
    type             TEXT NOT NULL,
    payload          JSONB NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','assigned','running','succeeded','retrying','dead','cancelled')),
    priority         INT NOT NULL DEFAULT 5,
    run_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    idempotency_key  TEXT,
    payload_hash     TEXT NOT NULL,
    attempts         INT NOT NULL DEFAULT 0,
    max_attempts     INT NOT NULL DEFAULT 3,
    worker_id        TEXT,
    lease_expires_at TIMESTAMPTZ,
    fence            BIGINT NOT NULL DEFAULT 0,
    result           JSONB,
    error            TEXT,
    created_by       TEXT REFERENCES users(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS jobs_idem_uq ON jobs(created_by, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS jobs_sched_idx ON jobs(status, run_at, priority);
CREATE INDEX IF NOT EXISTS jobs_hash_idx  ON jobs(payload_hash, created_at);
CREATE INDEX IF NOT EXISTS jobs_worker_idx ON jobs(worker_id) WHERE status IN ('assigned','running');

CREATE TABLE IF NOT EXISTS job_attempts (
    id          BIGSERIAL PRIMARY KEY,
    job_id      TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_no  INT NOT NULL,
    worker_id   TEXT NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    outcome     TEXT,            -- succeeded | failed | lost
    error       TEXT
);
CREATE INDEX IF NOT EXISTS attempts_job_idx ON job_attempts(job_id);

-- Demonstrates "no duplicate business operation": op_key is unique.
CREATE TABLE IF NOT EXISTS business_ops (
    op_key       TEXT PRIMARY KEY,
    job_id       TEXT NOT NULL,
    account      TEXT NOT NULL,
    amount_cents BIGINT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_log (
    id         BIGSERIAL PRIMARY KEY,
    actor      TEXT NOT NULL,
    action     TEXT NOT NULL,
    detail     TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
