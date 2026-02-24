# RPG Maker MV MMO Server

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

> ⚠️ **WARNING: EARLY DEVELOPMENT STAGE** ⚠️  
> This project is in early alpha stage. APIs may change without notice, database schemas may be modified, and critical bugs may exist. **Not recommended for production use.**

## What is this?

This is a **Multiplayer Online (MMO) game server** for [RPG Maker MV](https://www.rpgmakerweb.com/products/rpg-maker-mv) games. It transforms single-player RMMV games into real-time multiplayer experiences.

### Features

- 🎮 **Real-time Multiplayer**: Players can see and interact with each other on the same maps
- ⚔️ **Battle System**: Server-authoritative damage calculation with buff/debuff support
- 💬 **Chat System**: Global, party, and nearby chat with cooldown management
- 🤝 **Party System**: Form parties (up to 4 players) with shared experience
- 🏪 **Trade System**: Secure player-to-player item and gold trading
- 🏰 **Guild System**: Create and manage guilds with notices
- 📦 **Inventory Management**: Full inventory system with equipment slots
- 📬 **Mail System**: Send items and messages between players
- 🗺️ **Map System**: Seamless map transfers with passability validation
- 🤖 **NPC Events**: Server-side execution of RMMV event commands
- 🛡️ **Anti-cheat**: Speed hack detection, movement validation

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Client (RMMV Game)                       │
│              with MMO Client Plugin                         │
└────────────────────┬────────────────────────────────────────┘
                     │ WebSocket / HTTP
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                      MMO Server (Go)                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │ REST API │  │  WebSocket│  │  Game    │  │  World   │    │
│  │ (Gin)    │  │  Handler  │  │  Logic   │  │  Manager │    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │  Battle  │  │  Party   │  │  Trade   │  │   Chat   │    │
│  │  System  │  │  Manager │  │  Service │  │ Handler  │    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │
└────────────────────┬────────────────────────────────────────┘
                     │
         ┌───────────┴───────────┐
         ▼                       ▼
┌─────────────────┐    ┌─────────────────┐
│   Database      │    │     Cache       │
│ (SQLite/MySQL/  │    │ (Redis/Local)   │
│  Embedded XML)  │    │                 │
└─────────────────┘    └─────────────────┘
```

## Requirements

- **Go**: 1.21 or higher
- **RPG Maker MV**: A valid RMMV project (for game data)
- **Database**: SQLite (default) or MySQL 5.7+/8.0+
- **Cache** (optional): Redis 6.0+ or local cache

## Quick Start

### 1. Installation

```bash
# Clone the repository
git clone <repository-url>
cd mmo/server

# Download dependencies
go mod download

# Build the server
go build -o mmo-server.exe
```

### 2. Configuration

Copy the example configuration file and modify it for your environment:

```bash
cp config/config.yaml config/my-config.yaml
```

Edit `config/my-config.yaml`:

```yaml
server:
  port: 8080                    # Server port
  debug: true                   # Debug mode (disable in production)
  admin_key: "your-secret-key"  # Admin API key (change this!)
  game_dir: "path/to/rmmv/www"  # Path to your RMMV game's www folder

rpgmaker:
  data_path: "path/to/rmmv/www/data"  # RMMV data files
  img_path: "path/to/rmmv/www/img"    # RMMV image assets

database:
  mode: "sqlite"                # Options: sqlite, mysql, embedded_xml, embedded_memory
  sqlite_path: "./data/game.db" # SQLite database file path
  # For MySQL, uncomment and configure:
  # mysql_dsn: "user:password@tcp(127.0.0.1:3306)/rpg_mmo?charset=utf8mb4&parseTime=True"

security:
  jwt_secret: "your-jwt-secret-min-32-chars"  # JWT signing secret (must be >= 32 characters)
  jwt_ttl_h: "72h"                            # JWT token expiration
```

### 3. Prepare Your RMMV Game

1. Copy your RMMV project's `www` folder to a known location
2. Ensure the game can connect to your server (CORS configuration if needed)
3. Install the MMO client plugin (if available) in your game's `js/plugins` folder

### 4. Run the Server

```bash
# Using default config path
./mmo-server.exe

# Or specify custom config
./mmo-server.exe config/my-config.yaml
```

The server will start on the configured port (default: 8080).

### 5. Access the Game

- **Game Client**: Open `http://localhost:8080/` in a browser
- **API Documentation**: The server exposes REST APIs at `/api/*`
- **WebSocket Endpoint**: `ws://localhost:8080/ws?token=<jwt>`

## Configuration Reference

### Database Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `sqlite` | File-based SQLite database | Development, small servers |
| `mysql` | MySQL/MariaDB database | Production, high concurrency |
| `embedded_xml` | XML file storage (sqlexec) | Testing, legacy compatibility |
| `embedded_memory` | In-memory only (data lost on restart) | Unit testing |

### Game Configuration

```yaml
game:
  map_tick_ms: 50              # Map update interval (20 TPS)
  save_interval_s: 300         # Auto-save interval (seconds)
  max_party_size: 4            # Maximum party members
  pvp_enabled: false           # Enable PvP combat
  chat_nearby_range: 10        # Nearby chat range (tiles)
  global_chat_cooldown_s: 180  # Global chat cooldown (seconds)
```

### Security Configuration

⚠️ **Important**: Change default secrets before deploying!

```yaml
security:
  jwt_secret: "minimum-32-characters-long-secret-key"
  jwt_ttl_h: "72h"
  rate_limit_rps: 100          # Requests per second limit
  rate_limit_burst: 200        # Burst allowance
  allowed_origins: []          # CORS origins (empty = allow all in dev)
```

## API Overview

### Authentication

```bash
# Login (auto-registers if username doesn't exist)
POST /api/auth/login
{"username": "player", "password": "secret"}

# Response
{"token": "eyJ...", "account_id": 1}
```

### Characters

```bash
# List characters
GET /api/characters
Authorization: Bearer <token>

# Create character
POST /api/characters
{"name": "Hero", "class_id": 1, "walk_name": "Actor1", "face_name": "Actor1"}
```

### Admin Endpoints (requires `admin_key`)

```bash
# Get server metrics
GET /admin/metrics?key=<admin_key>

# List online players
GET /admin/players?key=<admin_key>

# Kick player
POST /admin/kick/<char_id>?key=<admin_key>
```

## WebSocket Protocol

Connect to `ws://host:port/ws?token=<jwt_token>`

### Client → Server Messages

| Type | Description |
|------|-------------|
| `enter_map` | Join a map with character |
| `player_move` | Send movement update |
| `map_transfer` | Request map change |
| `chat_send` | Send chat message |
| `party_invite` | Invite to party |
| `trade_request` | Request trade |
| `player_skill` | Use skill |

### Server → Client Messages

| Type | Description |
|------|-------------|
| `map_init` | Initial map state |
| `player_join` | Player entered map |
| `player_leave` | Player left map |
| `player_sync` | Position update |
| `chat_message` | Chat message |
| `npc_dialog` | NPC dialog text |
| `trade_request` | Incoming trade request |

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run with race detection
go test -race ./...

# Run specific package
go test ./game/trade/...
```

### Project Structure

```
mmo/server/
├── api/              # HTTP and WebSocket handlers
│   ├── rest/         # REST API endpoints
│   ├── sse/          # Server-Sent Events
│   └── ws/           # WebSocket handlers
├── game/             # Game logic
│   ├── battle/       # Damage calculation
│   ├── npc/          # NPC event execution
│   ├── party/        # Party management
│   ├── player/       # Player sessions
│   ├── skill/        # Skill system
│   ├── trade/        # Trading system
│   └── world/        # Map and world state
├── model/            # Database models
├── resource/         # RMMV data loading
├── middleware/       # HTTP middleware
├── config/           # Configuration
└── main.go           # Entry point
```

## Known Limitations & Issues

⚠️ **This is alpha software. Expect breaking changes.**

### Current Limitations

1. **Database Schema**: May change between versions without migration support
2. **Plugin Compatibility**: Not all RMMV plugins are supported
3. **Performance**: Not optimized for large-scale deployments (>1000 concurrent players)
4. **Documentation**: API documentation is incomplete
5. **Client Plugin**: Client-side plugin is under development

### Planned Features

- [ ] Full RMMV event command support
- [ ] Instance dungeons
- [ ] Guild wars
- [ ] Auction house
- [ ] Scripting API (Lua/JavaScript)
- [ ] Web-based admin dashboard
- [ ] Docker deployment support

## Troubleshooting

### Common Issues

**Q: Server starts but clients can't connect**
- Check `server.game_dir` points to valid RMMV www folder
- Verify firewall allows connections on the server port
- Check browser console for CORS errors

**Q: Database errors on startup**
- Ensure database directory exists and is writable
- For SQLite, check disk space
- For MySQL, verify connection string and permissions

**Q: Movement is jittery or laggy**
- Check `game.map_tick_ms` setting (lower = more responsive, more CPU)
- Verify network latency between client and server
- Check server CPU usage

### Getting Help

- Create an issue in the repository
- Check existing issues for similar problems
- Include server logs (run with `debug: true`)

## Contributing

This project is in early development. Contributions are welcome but please note:

1. Large architectural changes should be discussed first
2. Follow existing code style
3. Add tests for new features
4. Update documentation

## License

[MIT License](LICENSE)

## Acknowledgments

- Built with [Gin](https://github.com/gin-gonic/gin), [GORM](https://gorm.io/), and [Gorilla WebSocket](https://github.com/gorilla/websocket)
- Inspired by the RPG Maker MV community

---

**⚠️ DISCLAIMER**: This software is provided "as is" without warranty. Use at your own risk in production environments.
