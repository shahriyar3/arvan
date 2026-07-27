CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    balance BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE account_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    delta BIGINT NOT NULL,
    reason VARCHAR(50) NOT NULL,
    ref_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sms_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    to_number VARCHAR(20) NOT NULL,
    body TEXT NOT NULL,
    encoding VARCHAR(10) NOT NULL,
    message_type VARCHAR(20) NOT NULL DEFAULT 'standard',
    status VARCHAR(30) NOT NULL DEFAULT 'accepted',
    cost BIGINT NOT NULL,
    idempotency_key VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    locked_until TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    idempotency_key VARCHAR(255) NOT NULL,
    response_snapshot JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, idempotency_key)
);

CREATE TABLE processed_consumer_events (
    message_id UUID PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sms_messages_account_created ON sms_messages (account_id, created_at DESC);
CREATE INDEX idx_outbox_events_pending ON outbox_events (status, created_at) WHERE status = 'pending';
CREATE INDEX idx_account_ledger_account ON account_ledger (account_id, created_at DESC);
