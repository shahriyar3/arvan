# SMS Gateway

REST API for sending SMS with prepaid balance, async delivery, and delivery reports.

**Architecture & design:** see [SUBMISSION_EN.md](SUBMISSION_EN.md)

## Quick Start

```bash
cp .env.example .env
docker compose up -d
make migrate-up
make run-api
```

PostgreSQL listens on port **5433** (host) to avoid conflicts with other local databases.

## API

- Swagger UI: `http://localhost:8080/swagger/index.html` (Phase 6)
- Health: `http://localhost:8080/health/ready`

## Tests

```bash
make check
make test
```
