# SMS Gateway

REST API for sending SMS with prepaid balance, async delivery, and delivery reports.

**Architecture & design:** see [SUBMISSION_EN.md](SUBMISSION_EN.md)

## Quick Start (full stack)

```bash
cp .env.example .env
docker compose up -d
make migrate-up
make seed
```

The compose stack includes HAProxy (API `:8080`), two API replicas, outbox-relay, worker, mock operator, PostgreSQL, Redis, RabbitMQ, and Jaeger.

Verify:

```bash
curl http://localhost:8080/health/ready
./scripts/demo.sh
```

## Demo

```bash
make demo
# BASE_URL=http://localhost:8080 TOKEN_A=demo-token-account-a ./scripts/demo.sh
```

After `make seed`, demo accounts have **10,000** prepaid units:

- `demo-token-account-a`
- `demo-token-account-b`

## API

All `/v1/*` routes require header `X-Account-Token`.

Example:

```bash
curl -X POST http://localhost:8080/v1/account/topup \
  -H "X-Account-Token: demo-token-account-a" \
  -H "Content-Type: application/json" \
  -d '{"amount":1000}'

curl http://localhost:8080/v1/account/balance \
  -H "X-Account-Token: demo-token-account-a"

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

**Common errors:** `401` invalid/missing token; `402` insufficient balance; `429` rate limit exceeded.

## Observability

| Service | URL |
|---|---|
| Swagger UI | http://localhost:8080/swagger/index.html |
| Prometheus (API) | http://localhost:8080/metrics |
| Worker metrics | http://localhost:9091/metrics |
| Jaeger UI | http://localhost:16686 |
| Health | http://localhost:8080/health/ready |

PostgreSQL listens on port **5433** (host) to avoid conflicts with other local databases.

## Tests

```bash
make check              # lint + unit + race + integration
make test-integration   # PostgreSQL integration only
```

## Load Test

Requires [k6](https://k6.io/) and a running stack (`make seed` for balance):

```bash
make load-test
# default: 80 RPS — fits RATE_LIMIT_LIMIT=100 (one account)
make load-test LOAD_TARGET_RPS=50 LOAD_DURATION=15s
```

Default rate limit is **100 req/s per account** (Redis). `make load-test` uses 80 RPS so requests stay under the limit.

For ~1K RPS stress test, raise the limit first, then:

```bash
# Option A: bump limit in .env and restart API replicas
# RATE_LIMIT_LIMIT=2000
make load-test-stress

# Option B: disable rate limiting for the load-test run
# RATE_LIMIT_ENABLED=false
make load-test-stress
```

Script: `scripts/load/k6-send.js` — asserts p99 accept latency < 500ms and error rate < 1%.
