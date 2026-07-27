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
```

- Swagger UI: `http://localhost:8080/swagger/index.html` (Phase 6)
- Health: `http://localhost:8080/health/ready`

## Tests

```bash
make check
make test
```
