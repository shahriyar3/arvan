# SMS Gateway — Architecture & Implementation

> Engineering submission for ArvanCloud software developer challenge.

## Overview

This project implements a prepaid SMS Gateway REST API designed for high throughput (~100M messages/day) with burst tolerance. The API accepts send requests asynchronously (`202 Accepted`), deducts balance in a transactional path, and delivers messages through a RabbitMQ-backed pipeline with a transactional outbox pattern.

Key patterns: fire-and-forget acceptance, transactional outbox, idempotent API and consumers, read/write database split, express (OTP) priority queues, and circuit breaker on the operator adapter.

## Architecture

Phase 1 delivers the foundation: three Go binaries (`api`, `worker`, `outbox-relay`), PostgreSQL schema, Redis, and RabbitMQ via docker-compose. Full async pipeline, observability, and HAProxy are added in later phases.

```mermaid
flowchart LR
    Client --> API
    API --> PGPrimary[(PostgreSQL)]
    API --> Redis
    Relay --> PGPrimary
    Relay --> RMQ[RabbitMQ]
    RMQ --> Worker --> MockOp[Mock Operator]
```

## Key Design Decisions

- **Modular monolith** with separate binaries for API, worker, and outbox relay — simpler ops for a 7-day challenge while keeping clear scaling boundaries.
- **Transactional outbox** ensures balance deduction and message enqueue are atomic (implemented in Phase 3).
- **Pre-seeded `X-Account-Token`** instead of a full auth system, per challenge requirements.

## Scale Considerations

Target: ~1.2K msg/s average, 12–25K msg/s peak. API returns quickly; workers scale horizontally. Rate limiting and bulkhead pools are added in Phase 5.

## How to Run

```bash
cp .env.example .env
docker compose up -d
make migrate-up
make run-api
```

Health checks:

- Liveness: `GET http://localhost:8080/health/live`
- Readiness: `GET http://localhost:8080/health/ready`

Infrastructure (docker-compose):

| Service   | Port  |
|-----------|-------|
| PostgreSQL | 5433 |
| Redis      | 6379 |
| RabbitMQ   | 5672 (AMQP), 15672 (management UI) |

## API Documentation

Swagger UI will be available at `/swagger/index.html` after Phase 6.

## Testing

```bash
make check    # lint + unit tests + race detector
make test
```

## Trade-offs

- **Single-node PostgreSQL/Redis** in Phase 1 for fast local dev; replication and HAProxy added later.
- **Local docs** (`.local/`) stay out of Git; only this file and `README.md` are submitted as documentation.
