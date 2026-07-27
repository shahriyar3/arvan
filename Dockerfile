FROM golang:1.25-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/outbox-relay ./cmd/outbox-relay && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/mock-operator ./cmd/mock-operator

FROM alpine:3.20 AS api
RUN apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=builder /out/api /app/api
EXPOSE 8080
ENTRYPOINT ["/app/api"]

FROM alpine:3.20 AS worker
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/worker /app/worker
EXPOSE 9091
ENTRYPOINT ["/app/worker"]

FROM alpine:3.20 AS outbox-relay
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/outbox-relay /app/outbox-relay
EXPOSE 9092
ENTRYPOINT ["/app/outbox-relay"]

FROM alpine:3.20 AS mock-operator
RUN apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=builder /out/mock-operator /app/mock-operator
EXPOSE 8090
ENTRYPOINT ["/app/mock-operator"]

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/ /app/
EXPOSE 8080
ENTRYPOINT ["/app/api"]
