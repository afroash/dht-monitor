#!/bin/bash
set -e

echo "🔄 DHT Monitor - Phase 3B Manual Setup (SQLite Persistence)"
echo "============================================================"
echo ""
echo "⚠️  NOTE: This script is for manual setup only."
echo "    GitHub Actions will handle this automatically on deployment."
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo "❌ Please run as root (use sudo)"
    exit 1
fi

# Configuration
APP_DIR="/opt/dht-monitor"
SERVICE_USER="dht-monitor"
DB_DIR="/var/lib/dht-monitor"

echo "📊 Phase 3B Changes:"
echo "  • SQLite persistence layer"
echo "  • Historical data navigation"
echo "  • Grafana-style dashboard"
echo ""

read -p "Continue with manual setup? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Setup cancelled."
    exit 0
fi
echo ""

# ============================================
# 1. Create State Directory for Database
# ============================================
echo "📁 Setting up database directory..."

if [ ! -d "$DB_DIR" ]; then
    mkdir -p $DB_DIR
    echo "✅ Created: $DB_DIR"
else
    echo "✅ Directory exists: $DB_DIR"
fi

# Set correct ownership
chown $SERVICE_USER:$SERVICE_USER $DB_DIR
chmod 750 $DB_DIR
echo "✅ Permissions set: $SERVICE_USER:$SERVICE_USER (750)"

# ============================================
# 2. Update Systemd Service (Add StateDirectory)
# ============================================
echo ""
echo "🔧 Updating systemd service configuration..."

# Backup existing service file
if [ -f /etc/systemd/system/dht-server.service ]; then
    cp /etc/systemd/system/dht-server.service /etc/systemd/system/dht-server.service.backup-$(date +%Y%m%d-%H%M%S)
    echo "✅ Backed up existing service file"
fi

cat > /etc/systemd/system/dht-server.service << 'EOF'
[Unit]
Description=DHT Monitor Server
After=network.target
Wants=cloudflared.service

[Service]
Type=simple
User=dht-monitor
Group=dht-monitor
WorkingDirectory=/opt/dht-monitor
ExecStart=/opt/dht-monitor/bin/dht-server --config /opt/dht-monitor/configs/server.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=dht-server

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/dht-monitor/data /opt/dht-monitor/logs /var/lib/dht-monitor

# State Directory (for SQLite database)
StateDirectory=dht-monitor
StateDirectoryMode=0750

# Environment
Environment="SERVER_AUTH_TOKEN=REPLACE_ME_VIA_ENV_FILE"
Environment="ENV=production"

[Install]
WantedBy=multi-user.target
EOF

echo "✅ Updated systemd service (added StateDirectory + ENV=production)"

# ============================================
# 3. Update Server Config (if needed)
# ============================================
echo ""
echo "🔍 Checking server configuration..."

if grep -q "retention_days: 30" $APP_DIR/configs/server.yaml; then
    echo "✅ Config already has retention_days setting"
else
    echo "⚠️  Warning: Config might need retention_days setting"
    echo "   Current retention: Check your server.yaml manually"
fi

# Show current DB path from config
echo ""
echo "📋 Current configuration:"
grep -A 1 "storage:" $APP_DIR/configs/server.yaml | head -3

# ============================================
# 4. Reload Systemd
# ============================================
echo ""
echo "🔄 Reloading systemd daemon..."
systemctl daemon-reload
echo "✅ Systemd reloaded"

# ============================================
# 5. Check if database already exists
# ============================================
echo ""
echo "🔍 Checking for existing database..."

if [ -f "$DB_DIR/dht.db" ]; then
    DB_SIZE=$(du -h "$DB_DIR/dht.db" | cut -f1)
    echo "✅ Database exists: $DB_DIR/dht.db ($DB_SIZE)"
    echo "   Data will be preserved during update"
else
    echo "ℹ️  No existing database found"
    echo "   Fresh database will be created on first run"
fi

# ============================================
# Summary
# ============================================
echo ""
echo "=========================================="
echo "✅ Phase 3B Update Complete!"
echo "=========================================="
echo ""
echo "📋 What Changed:"
echo "  ✓ Database directory: $DB_DIR"
echo "  ✓ Systemd service updated with StateDirectory"
echo "  ✓ ENV=production added to service"
echo "  ✓ Permissions configured for SQLite"
echo ""
echo "📋 Next Steps:"
echo ""
echo "1. Deploy new binary via GitHub Actions:"
echo "   git push origin main"
echo ""
echo "2. OR manually deploy:"
echo "   cd /opt/dht-monitor"
echo "   git pull origin main"
echo "   go build -o bin/dht-server ./cmd/server"
echo ""
echo "3. Restart the service:"
echo "   sudo systemctl restart dht-server"
echo ""
echo "4. Monitor startup:"
echo "   sudo journalctl -u dht-server -f"
echo ""
echo "5. Verify database creation:"
echo "   ls -lh $DB_DIR/"
echo ""
echo "6. Test new endpoints:"
echo "   curl https://dht.afroash.com/api/current"
echo "   curl https://dht.afroash.com/api/stats"
echo "   curl https://dht.afroash.com/api/history"
echo ""
echo "7. Access new dashboard:"
echo "   https://dht.afroash.com"
echo ""
echo "📊 Database Info:"
echo "  • Location: $DB_DIR/dht.db"
echo "  • Retention: 7 days (configurable in code)"
echo "  • Auto-created: Yes"
echo "  • Schema: Auto-initialized"
echo ""
echo "🔧 Useful Commands:"
echo "  • View logs:      sudo journalctl -u dht-server -f"
echo "  • Check status:   sudo systemctl status dht-server"
echo "  • Query DB:       sudo sqlite3 $DB_DIR/dht.db 'SELECT COUNT(*) FROM readings;'"
echo "  • DB size:        sudo du -h $DB_DIR/dht.db"
echo ""