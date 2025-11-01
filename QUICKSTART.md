# Quick Start Guide

## 5-Minute Setup

### 1. Build the Bridge

```bash
cd erssi-lith-bridge
go build -o erssi-lith-bridge ./cmd/bridge
```

**Result**: `erssi-lith-bridge` binary (7.9MB)

### 2. Configure erssi

In your erssi IRC client:

```
/load fe-web
/set fe_web_port 9001
/set fe_web_password mypassword123
/set fe_web_bind_address 0.0.0.0
/save
```

### 3. Configure the Bridge

**Option A: Using .env file (Recommended)**

```bash
# Copy example configuration
cp .env.example .env

# Edit .env with your settings
nano .env
```

`.env` file contents:
```bash
ERSSI_URL=wss://your-server.com:9111
ERSSI_PASSWORD=mypassword123
LISTEN_ADDR=:9000
VERBOSE=true
```

**Option B: Using command-line flags**

```bash
./erssi-lith-bridge \
  -erssi ws://localhost:9001 \
  -password mypassword123 \
  -listen :9000 \
  -v
```

**Option C: Using environment variables**

```bash
export ERSSI_URL=ws://localhost:9001
export ERSSI_PASSWORD=mypassword123
export LISTEN_ADDR=:9000
export VERBOSE=true
./erssi-lith-bridge
```

**Priority order**: CLI flags > Environment variables > .env file > Defaults

### 4. Start the Bridge

```bash
./erssi-lith-bridge
```

**Expected output**:
```
INFO[2025-10-31 07:17:00] erssi-Lith Bridge v0.1.0
INFO[2025-10-31 07:17:00] erssi URL: ws://localhost:9001
INFO[2025-10-31 07:17:00] Listening on: :9000
INFO[2025-10-31 07:17:00] WeeChat protocol server listening on :9000
INFO[2025-10-31 07:17:00] Connected to erssi
INFO[2025-10-31 07:17:00] Bridge running, press Ctrl+C to stop...
```

### 5. Connect Lith

**In Lith app**:
- Host: `YOUR_SERVER_IP` (e.g., `192.168.1.100`)
- Port: `9000`
- SSL: **Off**
- Password: *(leave empty)*

**Tap "Connect"** ✅

---

## Architecture Diagram

```
┌─────────────┐
│ Lith Client │ (your phone/tablet)
└──────┬──────┘
       │ WeeChat protocol (TCP :9000)
       ↓
┌──────────────────┐
│ erssi-lith-bridge│ (this program)
└──────┬───────────┘
       │ erssi JSON (WebSocket :9001)
       ↓
┌──────────────┐
│ erssi/fe-web │ (your IRC client)
└──────────────┘
```

---

## Testing

### Test 1: Check Bridge Connection

```bash
# In another terminal
curl -v http://localhost:9000
```

Should see: **Connection refused** or **400 Bad Request** (good - means server is listening)

### Test 2: Check erssi WebSocket

```bash
# Using wscat (install: npm install -g wscat)
wscat -c ws://localhost:9001
```

Should connect and wait for JSON messages.

### Test 3: Send Test Message from Lith

Once connected in Lith:
1. Select a channel
2. Send: `Hello from Lith!`
3. Check erssi - message should appear

---

## Common Issues

### Bridge can't connect to erssi

**Error**: `Failed to connect to erssi: connection refused`

**Fix**:
```bash
# In erssi:
/module load fe-web
/set fe_web_port 9001
```

### Lith can't connect to bridge

**Error**: Connection timeout

**Fix**:
- Check firewall: `sudo ufw allow 9000` (Linux)
- Verify bridge is running: `ps aux | grep erssi-lith-bridge`
- Check correct IP address in Lith settings

### No messages appearing

**Fix**: Enable debug mode:
```bash
./erssi-lith-bridge -v
```

Look for:
- `Received message type=2` (from erssi)
- `WeeChat command: input` (from Lith)

---

## Next Steps

- **Production deployment**: See [USAGE.md](USAGE.md) for systemd/Docker setup
- **Remote access**: Add nginx reverse proxy with TLS
- **Development**: See [README.md](README.md) for architecture details

---

## Stopping the Bridge

**Terminal**: Press `Ctrl+C`

**Background process**:
```bash
pkill erssi-lith-bridge
```

---

## Build Info

- **Language**: Go 1.21+
- **Binary size**: ~8MB
- **Memory usage**: 20-50MB
- **Platforms**: Linux, macOS, Windows
- **License**: GPL v2+
