# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/outbox-relay ./cmd/outbox-relay

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/ /app/
EXPOSE 8080
ENTRYPOINT ["/app/api"]
