#!/bin/bash
set -e

echo "=================================="
echo "  MiniVault Production Setup"
echo "=================================="
echo ""

if [ "$EUID" -ne 0 ]; then
    echo "ERROR: Please run as root (sudo ./setup.sh)"
    exit 1
fi

DOMAIN="${DOMAIN:-}"
EMAIL="${EMAIL:-}"
WORKERS="${WORKERS:-3}"
DATA_DIR="${DATA_DIR:-/var/lib/minivault}"
INSTALL_DIR="/usr/local/bin"
SERVICE_USER="minivault"

while [[ $# -gt 0 ]]; do
    case $1 in
        --domain)
            DOMAIN="$2"
            shift 2
            ;;
        --email)
            EMAIL="$2"
            shift 2
            ;;
        --workers)
            WORKERS="$2"
            shift 2
            ;;
        --data-dir)
            DATA_DIR="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: sudo ./setup.sh [--domain yourdomain.com] [--email your@email.com] [--workers 3] [--data-dir /var/lib/minivault]"
            exit 1
            ;;
    esac
done

echo "Configuration:"
echo "  Domain: ${DOMAIN:-localhost (no SSL)}"
echo "  Email: ${EMAIL:-not set}"
echo "  Workers: $WORKERS"
echo "  Data Directory: $DATA_DIR"
echo ""

read -p "Continue with installation? [y/N] " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 0
fi

echo ""
echo "[1/8] Installing dependencies..."
apt-get update -qq
apt-get install -y golang nginx certbot python3-certbot-nginx ufw curl jq >/dev/null 2>&1

echo "[2/8] Building minivault..."
if [ ! -f "go.mod" ]; then
    echo "ERROR: Must run from minivault directory"
    exit 1
fi

go build -o minivault src/*.go
cp minivault "$INSTALL_DIR/minivault"
cp volume "$INSTALL_DIR/minivault-volume"
chmod +x "$INSTALL_DIR/minivault" "$INSTALL_DIR/minivault-volume"

echo "[3/8] Creating service user and directories..."
if ! id "$SERVICE_USER" >/dev/null 2>&1; then
    useradd -r -s /bin/false "$SERVICE_USER"
fi

mkdir -p "$DATA_DIR"
for i in $(seq 1 $WORKERS); do
    mkdir -p "$DATA_DIR/volume$((3000 + i))"
done
chown -R "$SERVICE_USER:$SERVICE_USER" "$DATA_DIR"

echo "[4/8] Creating systemd services..."

cat > /etc/systemd/system/minivault-volume@.service << 'EOF'
[Unit]
Description=MiniVault Volume %i
After=network.target

[Service]
Type=simple
User=minivault
Environment="PORT=%i"
ExecStart=/usr/local/bin/minivault-volume /var/lib/minivault/volume%i
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
for i in $(seq 1 $WORKERS); do
    PORT=$((3000 + i))
    systemctl enable "minivault-volume@$PORT" >/dev/null 2>&1
    systemctl restart "minivault-volume@$PORT"
done

sleep 2

echo "[5/8] Configuring Nginx..."

UPSTREAM_SERVERS=""
for i in $(seq 1 $WORKERS); do
    PORT=$((3000 + i))
    UPSTREAM_SERVERS="${UPSTREAM_SERVERS}    server 127.0.0.1:$PORT;\n"
done

cat > /etc/nginx/sites-available/minivault << EOF
upstream minivault_workers {
    least_conn;
$(echo -e "$UPSTREAM_SERVERS")
}

server {
    listen 80;
    server_name ${DOMAIN:-_};

    client_max_body_size 100M;

    location / {
        proxy_pass http://minivault_workers;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_http_version 1.1;
        proxy_set_header Connection "";

        proxy_connect_timeout 5s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    location /health {
        access_log off;
        return 200 "healthy\n";
        add_header Content-Type text/plain;
    }
}
EOF

ln -sf /etc/nginx/sites-available/minivault /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default
nginx -t
systemctl reload nginx

echo "[6/8] Configuring firewall..."
ufw --force enable
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force reload

if [ -n "$DOMAIN" ] && [ -n "$EMAIL" ]; then
    echo "[7/8] Setting up SSL with Let's Encrypt..."
    certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos -m "$EMAIL" --redirect
else
    echo "[7/8] Skipping SSL setup (no domain/email provided)"
fi

echo "[8/8] Running health checks..."
sleep 2

FAILED=0
for i in $(seq 1 $WORKERS); do
    PORT=$((3000 + i))
    if ! systemctl is-active --quiet "minivault-volume@$PORT"; then
        echo "  ✗ Worker on port $PORT is not running"
        FAILED=1
    else
        echo "  ✓ Worker on port $PORT is running"
    fi
done

if ! systemctl is-active --quiet nginx; then
    echo "  ✗ Nginx is not running"
    FAILED=1
else
    echo "  ✓ Nginx is running"
fi

if curl -sf http://localhost/health >/dev/null 2>&1; then
    echo "  ✓ HTTP endpoint responding"
else
    echo "  ✗ HTTP endpoint not responding"
    FAILED=1
fi

TEST_KEY="test-$(date +%s)"
TEST_VALUE="setup-test"
if curl -sf -X POST "http://localhost/$TEST_KEY" \
    -H "Content-Type: application/json" \
    -d "{\"value\": \"$TEST_VALUE\"}" >/dev/null 2>&1; then

    RETRIEVED=$(curl -sf "http://localhost/$TEST_KEY" | tr -d '"')
    if [ "$RETRIEVED" = "$TEST_VALUE" ]; then
        echo "  ✓ Data storage test passed"
        curl -sf -X DELETE "http://localhost/$TEST_KEY" >/dev/null 2>&1
    else
        echo "  ✗ Data storage test failed (retrieved: $RETRIEVED)"
        FAILED=1
    fi
else
    echo "  ✗ Data storage test failed"
    FAILED=1
fi

echo ""
echo "=================================="
if [ $FAILED -eq 0 ]; then
    echo "  ✓ Installation Complete!"
else
    echo "  ⚠ Installation completed with errors"
fi
echo "=================================="
echo ""

if [ -n "$DOMAIN" ]; then
    echo "Your minivault is now accessible at:"
    echo "  https://$DOMAIN"
    echo ""
    echo "Test it:"
    echo "  curl -X POST https://$DOMAIN/mykey -H 'Content-Type: application/json' -d '{\"value\": \"hello\"}'"
    echo "  curl https://$DOMAIN/mykey"
else
    echo "Your minivault is now accessible at:"
    echo "  http://YOUR_SERVER_IP"
    echo ""
    echo "Test it:"
    echo "  curl -X POST http://YOUR_SERVER_IP/mykey -H 'Content-Type: application/json' -d '{\"value\": \"hello\"}'"
    echo "  curl http://YOUR_SERVER_IP/mykey"
fi

echo ""
echo "Management commands:"
echo "  Check status:   systemctl status minivault-volume@3001"
echo "  View logs:      journalctl -u minivault-volume@3001 -f"
echo "  Restart worker: systemctl restart minivault-volume@3001"
echo "  Check all:      systemctl status 'minivault-volume@*'"
echo ""
echo "Data location: $DATA_DIR"
echo "Logs: journalctl -u minivault-volume@3001"

if [ $FAILED -ne 0 ]; then
    echo ""
    echo "⚠ Please check the errors above and fix any issues."
    exit 1
fi
