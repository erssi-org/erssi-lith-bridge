# erssi-Lith Bridge

WebSocket-to-WeeChat protocol bridge that allows Lith (WeeChat client) to connect to erssi/fe-web.

## Architecture

```
Lith Client (WeeChat protocol)
       ↕
  Bridge Daemon
       ↕
erssi/fe-web (JSON WebSocket)
```

## Components

### 1. erssi WebSocket Client
- Connects to erssi/fe-web WebSocket server
- Handles JSON message format (50 message types)
- Authentication and session management

### 2. WeeChat Protocol Server
- Accepts connections from Lith clients
- Implements WeeChat relay binary protocol
- Handles: handshake, init, hdata, input, sync, nicklist commands

### 3. Protocol Translator
- Bidirectional translation between erssi JSON ↔ WeeChat binary
- Message type mapping
- State synchronization

## Protocol Mapping

### erssi → WeeChat

| erssi Message Type | WeeChat Response |
|-------------------|------------------|
| WEB_MSG_MESSAGE | HData buffer_line |
| WEB_MSG_CHANNEL_JOIN | HData nicklist |
| WEB_MSG_NICKLIST | HData nicklist |
| WEB_MSG_SERVER_STATUS | Info/HashTable |
| WEB_MSG_STATE_DUMP | HData buffer:gui_buffers |

### WeeChat → erssi

| WeeChat Command | erssi JSON |
|----------------|-----------|
| input buffer ptr text | {"type":"command","text":"..."} |
| sync | Subscribe to all updates |
| hdata buffer:gui_buffers(*) | Request STATE_DUMP |
| nicklist | Request NICKLIST |

## Building

```bash
go build -o erssi-lith-bridge ./cmd/bridge
```

## Running

```bash
./erssi-lith-bridge
```

## Configuration

The bridge supports three configuration methods (in priority order):

### 1. Command-line Flags (Highest Priority)

```bash
./erssi-lith-bridge \
  -erssi wss://server.com:9111 \
  -password yourpassword \
  -listen :9000 \
  -v
```

### 2. Environment Variables

```bash
export ERSSI_URL=wss://server.com:9111
export ERSSI_PASSWORD=yourpassword
export LISTEN_ADDR=:9000
export VERBOSE=true
./erssi-lith-bridge
```

### 3. .env File (Recommended)

```bash
# Copy example and edit
cp .env.example .env
nano .env
```

`.env` file:
```bash
ERSSI_URL=wss://server.com:9111
ERSSI_PASSWORD=yourpassword
LISTEN_ADDR=:9000
VERBOSE=true
```

Then simply run:
```bash
./erssi-lith-bridge
```

**Configuration Variables:**
- `ERSSI_URL` / `-erssi` - erssi WebSocket URL (e.g., `wss://server:9111`)
- `ERSSI_PASSWORD` / `-password` - erssi WebSocket password
- `LISTEN_ADDR` / `-listen` - WeeChat protocol listen address (default: `:9000`)
- `VERBOSE` / `-v` - Enable verbose/debug logging (default: `false`)

## Development

Project structure:
```
.
├── cmd/
│   └── bridge/          # Main entry point
├── internal/
│   ├── erssi/           # erssi WebSocket client
│   ├── weechat/         # WeeChat protocol server
│   ├── translator/      # Protocol translation
│   └── bridge/          # Core bridge logic
├── pkg/
│   ├── erssiproto/      # erssi JSON protocol types
│   └── weechatproto/    # WeeChat binary protocol
└── README.md
```

## License

GPL v2+ (matching erssi and Lith licenses)
