#!/bin/bash
# Dijalankan otomatis oleh image postgres saat container pertama kali dibuat
# (semua file di /docker-entrypoint-initdb.d/ di-run sekali doang, pas volume masih kosong).
# Bikin 1 database per service ("database per service" pattern), tapi tetap
# di 1 container postgres biar infra-nya simpel.
set -e

for db in $(echo "$POSTGRES_MULTIPLE_DATABASES" | tr ',' ' '); do
	echo "Creating database '$db'"
	psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-EOSQL
		CREATE DATABASE $db;
EOSQL
done
