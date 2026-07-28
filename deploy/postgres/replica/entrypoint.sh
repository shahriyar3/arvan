#!/bin/bash
set -euo pipefail

PGDATA="${PGDATA:-/var/lib/postgresql/data}"

if [ ! -s "${PGDATA}/PG_VERSION" ]; then
  echo "Waiting for primary to accept connections..."
  until pg_isready -h postgres-primary -p 5432 -U "${POSTGRES_USER}" -d "${POSTGRES_DB}"; do
    sleep 2
  done

  echo "Cloning data directory from primary..."
  rm -rf "${PGDATA:?}"/*
  PGPASSWORD="${POSTGRES_REPLICATION_PASSWORD}" pg_basebackup \
    -h postgres-primary \
    -p 5432 \
    -U "${POSTGRES_REPLICATION_USER}" \
    -D "${PGDATA}" \
    -Fp -Xs -P -R \
    -S replication_slot

  chown -R postgres:postgres "${PGDATA}"
  chmod 700 "${PGDATA}"
fi

exec docker-entrypoint.sh postgres \
  -c hot_standby=on \
  -c hot_standby_feedback=on \
  -c max_connections=200
