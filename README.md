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
./erssi-lith-bridge -erssi ws://localhost:9001 -listen :9000
```

## Configuration

```bash
# Connect to erssi
ERSSI_URL=ws://localhost:9001
ERSSI_PASSWORD=yourpassword

# Listen for Lith clients
LISTEN_ADDR=:9000
```

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
