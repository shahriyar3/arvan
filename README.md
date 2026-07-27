# SMS Gateway

REST API for sending SMS with prepaid balance, async delivery, and delivery reports.

**Architecture & design:** see [SUBMISSION_EN.md](SUBMISSION_EN.md)

## Quick Start

```bash
cp .env.example .env
docker compose up -d
make migrate-up
make seed
make run-api
```

For full async delivery, also run `make run-mock-operator`, `make run-relay`, and `make run-worker` (see SUBMISSION_EN.md Phase 4).

With docker-compose, HAProxy load-balances two API replicas on port **8080** (Phase 5). Worker metrics: `http://localhost:9091/metrics`.

PostgreSQL listens on port **5433** (host) to avoid conflicts with other local databases.

## API

All `/v1/*` routes require header `X-Account-Token`. Demo tokens (after `make seed`):

- `demo-token-account-a`
- `demo-token-account-b`

Example:

```bash
curl -X POST http://localhost:8080/v1/account/topup \
  -H "X-Account-Token: demo-token-account-a" \
  -H "Content-Type: application/json" \
  -d '{"amount":1000}'

curl http://localhost:8080/v1/account/balance \
  -H "X-Account-Token: demo-token-account-a"

# Send SMS (costs 1 unit — top up first if balance is 0)
curl -X POST http://localhost:8080/v1/sms/send \
  -H "X-Account-Token: demo-token-account-a" \
  -H "Content-Type: application/json" \
  -d '{"to":"+989121234567","body":"Hello","message_type":"standard"}'
# → 202 {"message_id":"...","status":"accepted"}

# Optional: idempotency for safe retries
curl -X POST http://localhost:8080/v1/sms/send \
  -H "X-Account-Token: demo-token-account-a" \
  -H "Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000" \
  -H "Content-Type: application/json" \
  -d '{"to":"+989121234567","body":"Hello","message_type":"standard"}'
```

**Common errors:** `401` invalid/missing token (run `make seed`, use `demo-token-account-a` not a placeholder); `402` insufficient balance (top up first).

- Swagger UI: `http://localhost:8080/swagger/index.html` (Phase 6)
- Health: `http://localhost:8080/health/ready`

## Tests

```bash
make check
make test
```
