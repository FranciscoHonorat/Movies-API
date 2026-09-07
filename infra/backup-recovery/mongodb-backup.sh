#!/usr/bin/env bash
# Dumps the "movies" MongoDB database (movies + movie_jobs collections) to a
# timestamped, gzip-compressed archive. Local-only by default — zero cloud
# cost. Optionally also uploads to S3/LocalStack if BACKUP_S3_BUCKET is set;
# see infra/backup-recovery/backup-strategy.md for why this is a periodic
# dump rather than continuous/point-in-time backup.
#
# Usage:
#   MONGO_URI="mongodb://root:pass@localhost:27017/movies?authSource=admin" \
#     ./mongodb-backup.sh
#
# Env vars:
#   MONGO_URI        required. Same connection string the services use.
#   BACKUP_DIR        optional, default "./backups".
#   BACKUP_S3_BUCKET  optional. If set, also uploads via `aws s3 cp` (works
#                     against LocalStack when AWS_ENDPOINT_URL is exported —
#                     see infra/terraform/README.md).
set -euo pipefail

: "${MONGO_URI:?MONGO_URI is required, e.g. mongodb://root:pass@localhost:27017/movies?authSource=admin}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
ARCHIVE="${BACKUP_DIR}/movies-${TIMESTAMP}.archive.gz"

mkdir -p "${BACKUP_DIR}"

command -v mongodump >/dev/null 2>&1 || {
  echo "mongodump não encontrado. Instale mongodb-database-tools ou rode este script" >&2
  echo "dentro de um container que já tenha as ferramentas (ex.: mongo:8.0)." >&2
  exit 1
}

echo "Backing up via mongodump -> ${ARCHIVE}"
mongodump --uri="${MONGO_URI}" --archive="${ARCHIVE}" --gzip

if [ -n "${BACKUP_S3_BUCKET:-}" ]; then
  command -v aws >/dev/null 2>&1 || {
    echo "BACKUP_S3_BUCKET definido, mas o AWS CLI não está instalado — pulando upload." >&2
    exit 0
  }
  echo "Uploading to s3://${BACKUP_S3_BUCKET}/$(basename "${ARCHIVE}")"
  aws s3 cp "${ARCHIVE}" "s3://${BACKUP_S3_BUCKET}/$(basename "${ARCHIVE}")"
fi

echo "Backup concluído: ${ARCHIVE}"
