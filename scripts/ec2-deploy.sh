#!/usr/bin/env bash
# Deploy script for EC2: fetch config, run migrations, install binary, restart service.
# Invoke with: ./ec2-deploy.sh <S3_BUCKET> <AWS_REGION>
# Example: ./ec2-deploy.sh arcfind-builds us-east-1

set -e
export PATH="/usr/local/bin:/usr/bin:${PATH}"

if [ $# -lt 2 ]; then
  echo "Usage: $0 <S3_BUCKET> <AWS_REGION>" >&2
  exit 1
fi
BUCKET="$1"
REGION="$2"
APP_DIR="${APP_DIR:-/opt/go-sim-backend}"

SSM_PARAM_ENV="/arcfind/production/backend"
SSM_PARAM_FIREBASE="/arcfind/production/firebase-sa"

mkdir -p "$APP_DIR"

echo "Fetching .env from Parameter Store..."
aws ssm get-parameter \
  --name "$SSM_PARAM_ENV" \
  --with-decryption \
  --query "Parameter.Value" \
  --output text \
  --region "$REGION" | tee "$APP_DIR/.env" > /dev/null

echo "Fetching Firebase credentials from Parameter Store..."
aws ssm get-parameter \
  --name "$SSM_PARAM_FIREBASE" \
  --with-decryption \
  --query "Parameter.Value" \
  --output text \
  --region "$REGION" | tee "$APP_DIR/firebase-service-account.json" > /dev/null

echo "Syncing migrations from S3..."
aws s3 sync "s3://${BUCKET}/go-sim-backend/migrations/" "$APP_DIR/migrations/" --region "$REGION"

export PGPASSWORD
PGPASSWORD=$(grep '^DB_PASSWORD=' "$APP_DIR/.env" | cut -d= -f2-)
export DB_HOST
DB_HOST=$(grep '^DB_HOST=' "$APP_DIR/.env" | cut -d= -f2-)
export DB_PORT
DB_PORT=$(grep '^DB_PORT=' "$APP_DIR/.env" | cut -d= -f2-)
export DB_USER
DB_USER=$(grep '^DB_USER=' "$APP_DIR/.env" | cut -d= -f2-)
export DB_NAME
DB_NAME=$(grep '^DB_NAME=' "$APP_DIR/.env" | cut -d= -f2-)

echo "Running migrations..."
for f in $(ls -1 "$APP_DIR/migrations/"*.sql 2>/dev/null | sort); do
  psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f "$f" || exit 1
done

echo "Downloading app binary from S3..."
aws s3 cp "s3://${BUCKET}/go-sim-backend/app" "$APP_DIR/app" --region "$REGION"
chmod +x "$APP_DIR/app"

echo "Restarting go-sim-backend..."
sudo systemctl restart go-sim-backend

echo "Deploy completed successfully."
