# erssi-Lith Bridge - Usage Guide

## Overview

The erssi-Lith bridge allows you to connect **Lith** (a WeeChat mobile client) to your **erssi** IRC server by translating between WeeChat's binary relay protocol and erssi's JSON WebSocket protocol.

## Prerequisites

- **erssi** running with `fe-web` module enabled and WebSocket server listening
- **Go 1.21+** installed on your system (for building)
- **Lith** mobile/desktop app

## Building

```bash
# Clone or navigate to the bridge directory
cd erssi-lith-bridge

# Download dependencies
go mod download

# Build the bridge
go build -o erssi-lith-bridge ./cmd/bridge

# Or use make
make build
```

### Building for multiple platforms

```bash
make build-all
```

This creates binaries for:
- Linux (amd64)
- macOS (amd64, arm64)
- Windows (amd64)

## Configuration

The bridge can be configured via command-line flags:

```bash
./erssi-lith-bridge \
  -erssi ws://your-server:9001 \
  -password your_erssi_password \
  -listen :9000 \
  -v
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-erssi` | erssi WebSocket URL | `ws://localhost:9001` |
| `-password` | erssi WebSocket password | `` (empty) |
| `-listen` | Address for Lith to connect to | `:9000` |
| `-v` | Enable verbose debug logging | `false` |

### Environment Variables

Alternatively, use environment variables:

```bash
export ERSSI_URL=ws://your-server:9001
export ERSSI_PASSWORD=yourpassword
export LISTEN_ADDR=:9000

./erssi-lith-bridge
```

## Running

### 1. Start erssi with fe-web

Ensure erssi is running with the WebSocket server:

```bash
# In erssi
/load fe-web
/set fe_web_port 9001
/set fe_web_password yourpassword
/set fe_web_bind_address 127.0.0.1  # Or 0.0.0.0 for remote access
```

### 2. Start the bridge

```bash
./erssi-lith-bridge \
  -erssi ws://localhost:9001 \
  -password yourpassword \
  -listen :9000 \
  -v
```

You should see:

```
INFO[2025-10-30 23:59:00] erssi-Lith Bridge v0.1.0
INFO[2025-10-30 23:59:00] erssi URL: ws://localhost:9001
INFO[2025-10-30 23:59:00] Listening on: :9000
INFO[2025-10-30 23:59:00] Starting bridge...
INFO[2025-10-30 23:59:00] WeeChat protocol server listening on :9000
INFO[2025-10-30 23:59:00] Connecting to erssi at ws://localhost:9001
INFO[2025-10-30 23:59:00] Connected to erssi
INFO[2025-10-30 23:59:00] Bridge started successfully
INFO[2025-10-30 23:59:00] Bridge running, press Ctrl+C to stop...
```

### 3. Connect Lith

In Lith app settings:

- **Host**: IP address where bridge is running (e.g., `192.168.1.100` or `localhost`)
- **Port**: `9000` (or whatever you set with `-listen`)
- **Use SSL**: No (unless you add TLS support)
- **Password**: Leave blank (authentication is handled by erssi)

## Network Topology

```
[Lith Client]
    ↓ WeeChat binary protocol
    ↓ TCP/WebSocket :9000
[Bridge Daemon]
    ↓ erssi JSON protocol
    ↓ WebSocket :9001
[erssi/fe-web]
```

## Troubleshooting

### Bridge can't connect to erssi

```
ERROR Failed to connect to erssi: dial tcp: connection refused
```

**Solution**: Check that:
- erssi is running
- `fe-web` module is loaded (`/module load fe-web`)
- erssi WebSocket is listening (`/set fe_web_port`)
- Firewall allows the connection

### Lith can't connect to bridge

```
ERROR Accept error: connection refused
```

**Solution**: Check that:
- Bridge is running (`bridge running...` message)
- No firewall blocking port 9000
- Correct IP address in Lith settings

### No messages appearing in Lith

**Solution**: Enable debug logging:

```bash
./erssi-lith-bridge -v
```

Look for:
- `erssi message: type=X` - Messages from erssi
- `WeeChat command: X` - Commands from Lith
- `Sending message type=X` - Messages sent to Lith

### Authentication fails

```
ERROR authentication failed
```

**Solution**:
- Verify erssi password matches (`-password` flag)
- Check erssi logs for authentication errors

## Advanced Usage

### Running as a systemd service (Linux)

Create `/etc/systemd/system/erssi-lith-bridge.service`:

```ini
[Unit]
Description=erssi-Lith Bridge
After=network.target

[Service]
Type=simple
User=yourusername
ExecStart=/usr/local/bin/erssi-lith-bridge \
  -erssi ws://localhost:9001 \
  -password YOURPASSWORD \
  -listen :9000
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable erssi-lith-bridge
sudo systemctl start erssi-lith-bridge
sudo systemctl status erssi-lith-bridge
```

### Running in Docker

Create `Dockerfile`:

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o erssi-lith-bridge ./cmd/bridge

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/erssi-lith-bridge /usr/local/bin/
EXPOSE 9000
ENTRYPOINT ["erssi-lith-bridge"]
CMD ["-listen", ":9000"]
```

Build and run:

```bash
docker build -t erssi-lith-bridge .
docker run -d -p 9000:9000 erssi-lith-bridge \
  -erssi ws://host.docker.internal:9001 \
  -password yourpassword
```

### Reverse Proxy with TLS (nginx)

For secure remote access, put nginx in front:

```nginx
upstream lith_bridge {
    server 127.0.0.1:9000;
}

server {
    listen 443 ssl;
    server_name irc.yourdomain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://lith_bridge;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

## Known Limitations

### Current Implementation

This is version 0.1.0 with basic functionality:

✅ **Working**:
- Basic message relay (IRC → Lith)
- Buffer list
- Sending messages (Lith → IRC)
- Authentication
- Nicklist (partial)

⚠️ **Not Yet Implemented**:
- Hotlist (unread counts)
- Rich formatting/colors
- File uploads
- Some IRC events (kicks, bans, mode changes)
- Reconnection logic
- Compression (zlib/zstd)

### Performance Notes

- Binary is ~10-15MB (statically linked)
- Memory usage: ~20-50MB
- CPU usage: negligible (<1%)
- Supports multiple Lith clients simultaneously

## Contributing

See main README.md for development setup and contribution guidelines.

## Support

- **Issues**: https://github.com/your-repo/erssi-lith-bridge/issues
- **erssi**: https://erssi.org
- **Lith**: https://github.com/LithApp/Lith
