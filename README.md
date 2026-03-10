# RPG Maker MV MMO Framework

[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

> **WARNING: EARLY DEVELOPMENT STAGE**
> APIs may change without notice, database schemas may be modified, and critical bugs may exist. **Not recommended for production use.**

## What is this?

A **server-authoritative MMO framework** for [RPG Maker MV](https://www.rpgmakerweb.com/products/rpg-maker-mv) games. The server interprets RMMV event commands, manages all game state, and the client acts purely as a renderer — zero RMMV core file modifications, all via prototype hooks.

### Core Architecture: Dual Interpreter

The server executes **all** game logic (event commands, state changes, inventory, battles). The client receives results and renders them. This prevents cheating and ensures consistency across players.

- **A-type commands** (server-only): Switches, variables, gold, items, equipment, transfers, battles
- **B-type commands** (hybrid): Dialog, choices, move routes — server decides, client renders
- **C-type commands** (client-render): Screen effects, pictures, sounds — forwarded to client as `npc_effect`

### Features

- **Server-Authoritative Event Execution**: Full RMMV event command interpreter (90+ command codes)
- **TemplateEvent.js Support**: `<TE:name/id>` resolution, OverrideTarget, self-variables, `\TE{cond}` page conditions
- **Parallel Event Sync**: All parallel events (trigger=4) run in a single goroutine with frame-perfect synchronization
- **Battle System**: Server-authoritative damage calculation with buff/debuff, escape, and class transformation
- **Player State Isolation**: Per-player switches, variables, party, actors via PlayerStateVM
- **Real-time Multiplayer**: Players see each other on maps with position sync and anti-cheat
- **Chat/Party/Trade/Guild**: Full social systems with cooldown management
- **Inventory & Equipment**: Persistent inventory with 14-slot equipment system
- **Anti-cheat**: Speed hack detection, movement validation, EventMu locking

### Architecture Diagram

```
Client (RMMV + MMO Plugin)          Server (Go)
========================           ==========================
mmo-core.js    (WS, sync)         api/ws/     (WebSocket handlers)
mmo-npc.js     (dialog, effects)  game/npc/   (event executor)
mmo-battle.js  (battle render)    game/battle/(damage calc)
mmo-party.js   (party UI)        game/world/ (map rooms, NPC runtime)
mmo-trade.js   (trade UI)        game/player/(session, state VM)
mmo-chat.js    (chat UI)         resource/   (RMMV data loader)
...17 plugin files total          ...54 test files
        │                                │
        │◄──── WebSocket ────►│
        │                                │
        │  npc_dialog, npc_effect,       │
        │  map_init, battle_*,           │
        │  var_change, switch_change     │
        │                                │
                                  ┌──────┴──────┐
                                  │  Database   │
                                  │ SQLite/MySQL│
                                  └─────────────┘
```

## Requirements

- **Go**: 1.26+
- **RPG Maker MV**: A valid RMMV project (the `www` folder with data files)
- **Database**: SQLite (default) or MySQL 5.7+/8.0+
- **Cache** (optional): Redis 6.0+ or local cache

## Quick Start

### 1. Build

```bash
cd mmo/server
go mod download
go build -o mmo-server.exe
```

### 2. Configure

```bash
cp config/config.yaml config/my-config.yaml
```

Key settings in `config/my-config.yaml`:

```yaml
server:
  port: 8080
  debug: true
  admin_key: "your-secret-key"
  game_dir: "path/to/rmmv/www"    # Path to your RMMV game's www folder

rpgmaker:
  data_path: "path/to/rmmv/www/data"
  img_path: "path/to/rmmv/www/img"

database:
  mode: "sqlite"                    # sqlite, mysql, embedded_xml, embedded_memory
  sqlite_path: "./data/game.db"

security:
  jwt_secret: "your-jwt-secret-min-32-chars"
  jwt_ttl_h: "72h"
```

### 3. Install Client Plugin

Copy the contents of `mmo/client_plugin/` into your RMMV project's `js/plugins/` folder. The loader (`mmo-loader.js`) auto-loads all MMO plugin files. No RMMV core files are modified.

### 4. Run

```bash
./mmo-server.exe config/my-config.yaml
```

Access the game at `http://localhost:8080/`.

## Server-Side Event Execution

The executor (`game/npc/`) processes RMMV event commands server-side:

| Category | Commands | Server Behavior |
|----------|----------|-----------------|
| Dialog | 101/401 (ShowText), 102/402/403 (Choices) | Sends to client, blocks for ack/choice response |
| Flow Control | 111/411/412 (Conditional), 112/113/413 (Loop), 115 (Exit), 117 (CommonEvent), 118/119 (Label/Jump) | Full server-side evaluation |
| State | 121 (Switches), 122 (Variables), 123 (SelfSwitch) | Immediate apply + incremental sync to client |
| Inventory | 125 (Gold), 126 (Items), 127 (Weapons), 128 (Armors) | Persistent DB writes |
| Actor | 311-322 (HP/MP/State/EXP/Level/Param/Skill/Equip/Name/Class/Image) | Server-authoritative with DB persistence |
| Battle | 301 (BattleProcessing) | Creates server battle session, client renders |
| Transfer | 201 (MapTransfer) | Server validates and executes room change |
| Visual | 205 (MoveRoute), 230 (Wait), 221-225 (Screen effects), 231-235 (Pictures), 241-251 (Audio) | Forwarded as `npc_effect` to client |

### Parallel Events

Parallel events (trigger=4) run in a **single synchronized goroutine** per player, not separate goroutines. This ensures frame-perfect synchronization (e.g., player and NPC walking side-by-side on Map 110).

- Tick interval derived from slowest event's `moveSpeed`
- Wait frame countdown per event (RMMV-accurate)
- `map_id` tagging prevents stale effects after map transfer
- Player speed injection (`ROUTE_CHANGE_SPEED`) for synchronized movement

### Event Tagging

Tag events in RMMV editor notes to control server behavior:

- `<server:global>` — Shared state, all players see the same result
- `<server:player>` — Per-player state (default), each player gets independent execution
- `<TE:name>` or `<TE:id>` — TemplateEvent resolution from template map

### TemplateEvent.js Support

Server-side implementation of TemplateEvent.js plugin features:

- Template resolution (`<TE:name/id>`) from template map
- OverrideTarget with AutoOverride and `<OverRide>` tags
- Self-variables (indices 0-12 user, 13-17 reserved for RandomPos)
- `\TE{cond}` page conditions with full expression evaluation
- `TE_CALL_ORIGIN_EVENT`, `TE_CALL_MAP_EVENT`, `TE_SET_SELF_VARIABLE` plugin commands
- IntegrateNote, OriginalPages support

## WebSocket Protocol

Connect: `ws://host:port/ws?token=<jwt_token>`

### Client → Server

| Type | Description |
|------|-------------|
| `enter_map` | Join map with character selection |
| `player_move` | Position update (x, y, dir) |
| `map_transfer` | Request map change |
| `npc_interact` | Talk to NPC (event_id) |
| `npc_dialog_ack` | Acknowledge dialog message |
| `npc_choice` | Choose dialog option |
| `npc_effect_ack` | Acknowledge visual effect (with optional position sync) |
| `chat_send` | Send chat message |
| `party_invite` | Invite to party |
| `trade_request` | Request trade |
| `player_skill` | Use skill |
| `battle_action` | Submit battle command |

### Server → Client

| Type | Description |
|------|-------------|
| `map_init` | Full map state (players, NPCs, switches, variables, equipment) |
| `player_join` / `player_leave` | Player enter/exit map |
| `player_sync` | Position broadcast |
| `npc_dialog` | Dialog text with face graphic |
| `npc_choices` | Choice prompt |
| `npc_effect` | Visual command to execute (code, params, map_id) |
| `npc_dialog_end` | Event execution complete |
| `event_end` | Event interaction ended (including lock rejection) |
| `var_change` / `switch_change` | Incremental state sync |
| `battle_start` / `battle_result` | Battle lifecycle |
| `move_reject` | Movement rejected (anti-cheat) |

## Development

### Running Tests

```bash
cd mmo/server

# Run all unit tests (no game data required)
go test ./...

# Run with race detection
go test -race ./...

# Run specific package tests
go test -v ./game/npc/         # Event executor + parallel events
go test -v ./game/battle/      # Battle system
go test -v ./api/ws/           # WebSocket handlers
go test -v ./game/world/       # Map rooms, NPC runtime

# Run integration tests (requires ProjectB game data)
go test -tags=projectb -v -timeout 300s ./integration/ -run TestProjectB
```

### Project Structure

```
mmo/
├── server/                    # Go server (122 source files, 54 test files)
│   ├── api/
│   │   ├── rest/              # REST API (auth, characters, admin)
│   │   ├── sse/               # Server-Sent Events
│   │   └── ws/                # WebSocket handlers (game, NPC, battle)
│   ├── game/
│   │   ├── ai/                # Battle AI
│   │   ├── battle/            # Damage calculation, buffs, turn management
│   │   ├── chat/              # Chat system
│   │   ├── npc/               # Event executor, parallel events, TemplateEvent
│   │   ├── party/             # Party management
│   │   ├── player/            # Player sessions, PlayerStateVM
│   │   ├── quest/             # Quest tracking
│   │   ├── script/            # Goja JS VM for script evaluation
│   │   ├── skill/             # Skill system
│   │   ├── trade/             # Trading system
│   │   └── world/             # Map rooms, NPC runtime, page selection
│   ├── model/                 # GORM database models
│   ├── resource/              # RMMV data loader (maps, actors, items, plugins)
│   ├── middleware/             # HTTP middleware (JWT, rate limit, CORS)
│   ├── config/                # Configuration
│   ├── integration/           # Integration tests (ProjectB flows)
│   └── main.go
├── client_plugin/             # RMMV client plugin (17 JS files)
│   ├── mmo-loader.js          # Auto-loader for all MMO plugins
│   ├── mmo-core.js            # WebSocket, position sync, state gates
│   ├── mmo-npc.js             # Dialog rendering, effect execution
│   ├── mmo-battle.js          # Battle UI bridge
│   ├── mmo-party.js           # Party UI
│   ├── mmo-trade.js           # Trade UI
│   ├── mmo-chat.js            # Chat UI
│   └── ...
├── e2e/                       # Playwright E2E tests
└── docs/                      # Design documents
    ├── 服务器权威架构改造方案.md
    └── 游戏设计者使用指南.md
```

## Configuration Reference

### Database Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `sqlite` | File-based SQLite | Development, small servers |
| `mysql` | MySQL/MariaDB | Production, high concurrency |
| `embedded_xml` | XML file storage | Testing, legacy compatibility |
| `embedded_memory` | In-memory only | Unit testing |

### Game Settings

```yaml
game:
  map_tick_ms: 50              # Map update interval (20 TPS)
  save_interval_s: 300         # Auto-save interval
  max_party_size: 4
  pvp_enabled: false
  chat_nearby_range: 10        # Nearby chat range (tiles)
  global_chat_cooldown_s: 180
```

### Game-Specific Configuration (MMOConfig.json)

The framework separates all game-specific logic from the core engine. Each RMMV project provides its own configuration in `www/data/MMOConfig.json`. If the file is absent, the framework runs with no game-specific behavior (no blocked plugins, no broadcast sync, no server-exec plugins, no time period computation).

```jsonc
{
  // Plugin commands that should NOT be forwarded to the client.
  // Use this for portrait/UI plugins that the client manages independently,
  // or calculation plugins that are handled server-side.
  "blockedPluginCmds": [
    "CallStand", "CallStandForce",
    "EraceStand", "EraceStand1",
    "CallCutin", "EraceCutin",
    "CallAM",
    "CulPartLV", "CulLustLV", "CulMiasmaLV"
  ],

  // Plugin commands executed server-side via Goja JS VM.
  // Each entry maps a plugin command name to its configuration.
  "serverExecPlugins": {
    "CulSkillEffect": {
      "scriptFile": "js/plugins/CulSkillEffect_server.js",  // relative to game root
      "timeout": 200,                // execution timeout in ms
      "injectActors": true,          // inject $gameActors into VM
      "injectDataArrays": true,      // inject $dataArmors, $dataWeapons, etc.
      "injectPlayerVars": false,     // inject __playerLevel, __gold, __classId
      "tagSkillListRange": [21, 122] // TagSkillList post-processing range
    },
    "ParaCheck": {
      "scriptFile": "js/plugins/ParaCheck_server.js",
      "timeout": 200,
      "injectActors": true,
      "injectDataArrays": true,
      "injectPlayerVars": true
    }
  },

  // Variables broadcast to ALL connected clients when changed (global state).
  // Typically time/weather variables that all players should see simultaneously.
  "broadcastVariables": [202, 203, 204, 205, 206, 207, 211],

  // Switches broadcast to ALL connected clients when changed.
  "broadcastSwitches": [11, 12, 20, 31, 53, 54, 55, 56, 57, 58, 87, 89, 103, 104],

  // Time period computation: derives a "period" variable from an "hour" variable.
  // The server computes this on map entry so maps without autoruns still get correct time.
  "timePeriod": {
    "hourVar": 204,      // variable ID containing current hour (0-23)
    "periodVar": 206,    // variable ID to write the period value
    "ranges": [          // first match wins (hour < maxHour)
      { "maxHour": 5,  "period": 6 },
      { "maxHour": 7,  "period": 1 },
      { "maxHour": 9,  "period": 2 },
      { "maxHour": 17, "period": 3 },
      { "maxHour": 19, "period": 4 },
      { "maxHour": 22, "period": 5 },
      { "maxHour": 99, "period": 6 }
    ]
  },

  // Script line prefixes allowed to be forwarded to the client (code 355).
  // Lines not matching any prefix or $gameScreen method are silently dropped.
  "safeScriptPrefixes": ["AudioManager."],

  // $gameScreen methods allowed to be forwarded (explicit whitelist).
  "safeScreenMethods": [
    "movePicture", "erasePicture", "showPicture", "picture",
    "tintPicture", "rotatePicture",
    "startFadeOut", "startFadeIn", "startTint", "startFlash", "startShake",
    "setWeather", "showBalloon",
    "startZoom", "setZoom", "clearZoom",
    "updateFadeOut", "updateFadeIn", "clearPictures"
  ]
}
```

#### Server-Exec Plugin Scripts

Plugins listed in `serverExecPlugins` have their JS executed server-side in a sandboxed Goja VM. The script file path is relative to the game root (parent of `www/data/`). The VM provides:

- `$gameVariables` / `$gameSwitches` — read/write game state (mutations tracked and synced to client)
- `$gameActors` — read equipment slots (if `injectActors: true`)
- `$dataArmors` / `$dataWeapons` / `$dataSkills` / `$dataItems` — RMMV data arrays with parsed meta (if `injectDataArrays: true`)
- `__playerLevel` / `__gold` / `__classId` — player-specific values (if `injectPlayerVars: true`)
- `Math` — standard JS Math object

To add a new server-exec plugin: create the JS file, add an entry to `serverExecPlugins`, and implement a handler case in `executor_dispatch.go:execServerPlugin()`.

### Client Initialization (InitState.json)

Optional file `www/data/InitState.json` provides client-side initialization data sent via `map_init`. This replaces hardcoded actor property setup:

```jsonc
{
  // Switches to set on client after map load
  "switches": { "85": true },

  // Switches to reset at event_end (e.g., to re-trigger parallel CEs)
  "resetSwitches": { "15": false },

  // Actor property initialization (custom plugin properties)
  "actorProps": {
    "1": {
      "EXT": { "mouth": 0, "nipple": 0, "clit": 0, "vagina": 0, "anus": 0 },
      "QTE": {},
      "hairTone": [0, 0, 0, 0, 0]
    }
  }
}
```

### TagSkillList.json

Optional file `www/data/TagSkillList.json` defines mappings for equipment stat base value computation:

```json
{
  "21": { "BaseVar": 1091, "AddVar": 4221, "BaseNum": 10 },
  "22": { "BaseVar": 1092, "AddVar": 4222, "BaseNum": 20 }
}
```

Each entry: `v[BaseVar] = BaseNum + v[AddVar]` — applied after the CulSkillEffect JS finishes accumulating equipment effects.

### Character Initialization (Automatic)

Character creation stats (starting equipment, switches, variables) are **automatically extracted** from Common Event 1 at server startup — no config needed. The server parses CE 1 and all its sub-CE calls, extracting:

- Code 121 (Control Switches) — initial switch values
- Code 122 (Control Variables) — initial variable values
- Code 356 (Plugin Commands) — EquipChange commands for starting equipment

## Known Limitations

1. **Database Schema**: May change between versions without migration
2. **Plugin Compatibility**: Not all RMMV plugins are supported server-side
3. **Performance**: Not optimized for >1000 concurrent players
4. **TemplateEvent.js**: ~95% complete; some client-only features not on server (KeepEventId, message \sv[n] display)

## Tech Stack

- **Server**: Go 1.26, [Gin](https://github.com/gin-gonic/gin), [GORM](https://gorm.io/), [Gorilla WebSocket](https://github.com/gorilla/websocket), [Goja](https://github.com/nicholasgasior/goja) (JS VM), [Zap](https://github.com/uber-go/zap) (logging)
- **Client**: Vanilla JS (RMMV plugin format), zero core modifications
- **Auth**: JWT with configurable TTL
- **Testing**: Go testing + testify, 54 test files, integration tests with real game data

## License

[MIT License](LICENSE)

---

**DISCLAIMER**: This software is provided "as is" without warranty. Use at your own risk in production environments.
