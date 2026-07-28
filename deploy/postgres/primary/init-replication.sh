#!/bin/bash
set -euo pipefail

{
  echo "host replication ${POSTGRES_REPLICATION_USER} all scram-sha-256"
} >> "${PGDATA}/pg_hba.conf"

psql -v ON_ERROR_STOP=1 --username "${POSTGRES_USER}" --dbname "${POSTGRES_DB}" <<-EOSQL
    CREATE USER ${POSTGRES_REPLICATION_USER} WITH REPLICATION ENCRYPTED PASSWORD '${POSTGRES_REPLICATION_PASSWORD}';
    SELECT pg_create_physical_replication_slot('replication_slot');
EOSQL
