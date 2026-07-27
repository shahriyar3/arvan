DROP INDEX IF EXISTS idx_account_ledger_account;
DROP INDEX IF EXISTS idx_outbox_events_pending;
DROP INDEX IF EXISTS idx_sms_messages_account_created;

DROP TABLE IF EXISTS processed_consumer_events;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS sms_messages;
DROP TABLE IF EXISTS account_ledger;
DROP TABLE IF EXISTS accounts;

DROP EXTENSION IF EXISTS "pgcrypto";
