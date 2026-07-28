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

The compose stack includes HAProxy (API `:8080`), two API replicas, outbox-relay, worker, mock operator, PostgreSQL primary + read replica, Redis, RabbitMQ, Jaeger, Prometheus, and Grafana.

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
| Grafana dashboard | http://localhost:3000 (admin / admin) |
| Prometheus UI | http://localhost:9090 |
| Swagger UI | http://localhost:8080/swagger/index.html |
| Prometheus (API) | http://localhost:8080/metrics |
| Worker metrics | http://localhost:9091/metrics |
| Outbox-relay metrics | http://localhost:9092/metrics |
| Jaeger UI | http://localhost:16686 |
| Health (primary + replica ping) | http://localhost:8080/health/ready |

Grafana loads a custom **SMS Gateway** dashboard from `deploy/grafana/dashboards/sms-gateway.json` (API accept rate/latency, outbox backlog, express SLA, circuit breaker). Prometheus scrapes all three metric endpoints every 15s.

PostgreSQL primary listens on port **5433** and the read replica on **5434** (host) to avoid conflicts with other local databases. API read queries (SMS list/get, ledger) use the replica via GORM dbresolver; writes and balance reads use the primary.

**Docker build fails with `DNS: transient error`?** That is a temporary network/DNS issue reaching Alpine package mirrors. Retry `docker compose up -d --build`. The Dockerfile retries `apk add` automatically; if it still fails, check Docker Desktop DNS settings or your VPN/proxy.

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
