FROM golang:1.25-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/outbox-relay ./cmd/outbox-relay && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/mock-operator ./cmd/mock-operator

FROM alpine:3.20 AS runtime-base
RUN apk add --no-cache ca-certificates || \
    (sleep 5 && apk add --no-cache ca-certificates) || \
    (sleep 10 && apk add --no-cache ca-certificates) || \
    (sleep 15 && apk add --no-cache ca-certificates)

FROM runtime-base AS api
RUN apk add --no-cache wget || \
    (sleep 5 && apk add --no-cache wget) || \
    (sleep 10 && apk add --no-cache wget)
WORKDIR /app
COPY --from=builder /out/api /app/api
EXPOSE 8080
ENTRYPOINT ["/app/api"]

FROM runtime-base AS worker
WORKDIR /app
COPY --from=builder /out/worker /app/worker
EXPOSE 9091
ENTRYPOINT ["/app/worker"]

FROM runtime-base AS outbox-relay
WORKDIR /app
COPY --from=builder /out/outbox-relay /app/outbox-relay
EXPOSE 9092
ENTRYPOINT ["/app/outbox-relay"]

FROM runtime-base AS mock-operator
RUN apk add --no-cache wget || \
    (sleep 5 && apk add --no-cache wget) || \
    (sleep 10 && apk add --no-cache wget)
WORKDIR /app
COPY --from=builder /out/mock-operator /app/mock-operator
EXPOSE 8090
ENTRYPOINT ["/app/mock-operator"]

FROM runtime-base AS default
WORKDIR /app
COPY --from=builder /out/ /app/
EXPOSE 8080
ENTRYPOINT ["/app/api"]
