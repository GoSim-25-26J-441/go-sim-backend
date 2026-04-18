#!/usr/bin/env bash
# Deploy script for EC2: fetch config, run migrations, install binary, restart service.
# Invoke with: ./ec2-deploy.sh <S3_BUCKET> <AWS_REGION>
# Example: ./ec2-deploy.sh arcfind-builds us-east-1
#
# Optional env:
#   APP_DIR          install directory (default /opt/go-sim-backend)
#   SYSTEMD_SERVICE  unit name without path (default go-sim-backend)
#   DEPLOY_USER      user:group for the service (default ec2-user)
#   SKIP_NGINX       set to 1 to skip nginx vhost logic entirely
#                    When nginx is present, api.microsim.dev.conf is copied from S3 only if
#                    /etc/nginx/conf.d/api.microsim.dev.conf does not already exist.

set -e
export PATH="/usr/local/bin:/usr/bin:${PATH}"

if [ $# -lt 2 ]; then
  echo "Usage: $0 <S3_BUCKET> <AWS_REGION>" >&2
  exit 1
fi
BUCKET="$1"
REGION="$2"
APP_DIR="${APP_DIR:-/opt/go-sim-backend}"
SYSTEMD_SERVICE="${SYSTEMD_SERVICE:-go-sim-backend}"
UNIT_FILE="/etc/systemd/system/${SYSTEMD_SERVICE}.service"
DEPLOY_USER="${DEPLOY_USER:-ec2-user}"

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

if [ ! -f "$UNIT_FILE" ]; then
  echo "Systemd unit not found; creating ${UNIT_FILE}..."
  sudo tee "$UNIT_FILE" > /dev/null <<EOF
[Unit]
Description=go-sim-backend API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${DEPLOY_USER}
Group=${DEPLOY_USER}
WorkingDirectory=${APP_DIR}
# godotenv.Load() reads .env from WorkingDirectory
ExecStart=${APP_DIR}/app
Restart=on-failure
RestartSec=5
KillSignal=SIGTERM
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
EOF
  sudo systemctl daemon-reload
  sudo systemctl enable "${SYSTEMD_SERVICE}"
fi

echo "Restarting ${SYSTEMD_SERVICE}..."
sudo systemctl restart "${SYSTEMD_SERVICE}"

if [ "${SKIP_NGINX:-0}" != "1" ] && command -v nginx >/dev/null 2>&1; then
  NGINX_DST="/etc/nginx/conf.d/api.microsim.dev.conf"
  if [ -f "$NGINX_DST" ]; then
    echo "Nginx vhost already present at ${NGINX_DST}; skipping api.microsim.dev.conf install."
  else
    echo "Installing nginx vhost (api.microsim.dev)..."
    aws s3 cp "s3://${BUCKET}/go-sim-backend/nginx/api.microsim.dev.conf" /tmp/api.microsim.dev.conf --region "$REGION"
    sudo cp /tmp/api.microsim.dev.conf "$NGINX_DST"
    sudo chmod 644 "$NGINX_DST"
    sudo nginx -t
    sudo systemctl reload nginx
    echo "Nginx vhost installed and nginx reloaded."
  fi
elif [ "${SKIP_NGINX:-0}" != "1" ]; then
  echo "Warning: nginx not found in PATH; skipping vhost install. Set SKIP_NGINX=1 to silence this."
fi

echo "Deploy completed successfully."
