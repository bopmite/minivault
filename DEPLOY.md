# Deployment Guide

## Single Server Setup

```bash
git clone https://github.com/bopmite/minivault
cd minivault
sudo ./setup.sh --domain yourdomain.com --email admin@yourdomain.com
```

Access at `https://yourdomain.com`

### What Gets Installed

```
INTERNET (80/443) → NGINX → WORKERS (3001,3002,3003) → /var/lib/minivault
```

- Nginx reverse proxy with SSL
- 3 worker nodes as systemd services
- Automatic SSL via Let's Encrypt
- Firewall configured (UFW)

### Options

```bash
sudo ./setup.sh --workers 5
sudo ./setup.sh --data-dir /mnt/storage/minivault
sudo ./setup.sh
```

### Management

```bash
systemctl status 'minivault-volume@*'
journalctl -u minivault-volume@3001 -f
systemctl restart minivault-volume@3001
```

Add workers:

```bash
sudo mkdir -p /var/lib/minivault/volume3005
sudo chown minivault:minivault /var/lib/minivault/volume3005
sudo systemctl enable minivault-volume@3005
sudo systemctl start minivault-volume@3005
```

Edit `/etc/nginx/sites-available/minivault` and add `server 127.0.0.1:3005;`, then reload nginx.

Update:

```bash
cd minivault
git pull
go build -o minivault src/*.go
sudo cp minivault /usr/local/bin/
sudo systemctl restart 'minivault-volume@*'
```

## Monitoring

```bash
tail -f /var/log/nginx/access.log
journalctl -u 'minivault-volume@*' -f
du -sh /var/lib/minivault/*
```

## Backup

```bash
sudo systemctl stop 'minivault-volume@*'
sudo tar -czf minivault-backup-$(date +%Y%m%d).tar.gz /var/lib/minivault/
sudo systemctl start 'minivault-volume@*'
```

Restore:

```bash
sudo systemctl stop 'minivault-volume@*'
sudo rm -rf /var/lib/minivault/*
sudo tar -xzf minivault-backup-YYYYMMDD.tar.gz -C /
sudo chown -R minivault:minivault /var/lib/minivault
sudo systemctl start 'minivault-volume@*'
```

## Troubleshooting

```bash
journalctl -u minivault-volume@3001 -n 50
sudo nginx -t
tail -f /var/log/nginx/error.log
sudo lsof -i :3001
sudo chown -R minivault:minivault /var/lib/minivault
```

## Performance Tuning

Increase file descriptors in `/etc/systemd/system/minivault-volume@.service`:

```ini
[Service]
LimitNOFILE=65535
```

Nginx config:

```nginx
worker_processes auto;
worker_connections 4096;

upstream minivault_workers {
    least_conn;
    keepalive 32;
}
```

Kernel tuning in `/etc/sysctl.conf`:

```
net.core.somaxconn = 4096
net.ipv4.tcp_max_syn_backlog = 4096
net.ipv4.ip_local_port_range = 1024 65535
```

Apply with `sudo sysctl -p`

## Uninstall

```bash
sudo systemctl stop 'minivault-volume@*'
sudo systemctl disable 'minivault-volume@*'
sudo rm /etc/systemd/system/minivault-volume@.service
sudo rm /usr/local/bin/minivault
sudo rm /usr/local/bin/minivault-volume
sudo rm -rf /var/lib/minivault
sudo rm /etc/nginx/sites-available/minivault
sudo rm /etc/nginx/sites-enabled/minivault
sudo userdel minivault
sudo systemctl daemon-reload
sudo systemctl reload nginx
```
