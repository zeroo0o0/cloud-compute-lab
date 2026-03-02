# Cloud Compute Book / Warzone Multiplayer Battle Game

This project contains two implemented local network multiplayer battle games: a C++ version and a Go version.

## Project Overview

This is a small-scale game project supporting multiple players battling on the same local network. Players can move on the map, pick up weapons, and attack opponents with the goal of eliminating other players. The game includes a complete account and statistics system with persistent player data storage.

## Project Structure

```
warzone/
├── c++/                     # C++ Implementation (v3.0 Monolithic Prototype)
│   ├── client.cpp         # Client program
│   ├── server.cpp         # Server program
│   ├── database.h         # Database operations and password hashing
│   ├── protocol.h         # Network protocol and game parameter definitions
│   ├── Makefile           # Build configuration
│   └── README.md          # C++ version documentation
│
└── go/                      # Go Language Implementation (TUI Multiplayer Battle Demo)
    ├── cmd/
    │   ├── client/        # Client program
    │   └── server/        # Server program
    ├── internal/
    │   ├── client/        # Client core logic
    │   │   ├── backend.go
    │   │   ├── net_backend.go
    │   │   └── tui/       # Terminal UI implementation
    │   ├── proto/         # Protocol definitions
    │   └── server/        # Server core logic and storage
    ├── game.db              # SQLite database file
    ├── go.mod
    └── README.md            # Go version documentation
```

## Key Features

### General Features

- **Multiplayer Online**: Supports multiple players battling on the same local network
- **Account System**: Player registration and login with persistent data storage
- **Statistics System**: Tracks player wins, kills, deaths, and other metrics
- **Real-time State Synchronization**: Server broadcasts game state to all clients

### Gameplay Mechanics

- **Movement Control**: Players move on the map using arrow keys
- **Weapon System**: Weapons spawn randomly on the map; picking them up grants attack capabilities
- **Combat System**: Players can attack opponents within their field of view
- **Health System**: Each player has a health value that decreases when attacked

## C++ Version (v3.0)

### Compilation and Execution

```bash
cd warzone/c++
make
```

### Starting the Server

```bash
./server [port]
# Default port: 8888
```

### Starting the Client

```bash
./client [server IP] [port]
# Default connection: localhost:8888
```

### Controls

- **Arrow Keys**: Move
- **Spacebar**: Attack
- **Q**: View statistics
- **ESC**: Exit game

### Configurable Parameters

Modify the following parameters in `protocol.h`:

- `MAX_PLAYERS`: Maximum number of players
- `MAP_W / MAP_H`: Map dimensions
- `MAX_HEALTH`: Maximum health
- `ATTACK_DAMAGE`: Normal attack damage
- `POWER_DAMAGE`: Powerful attack damage
- `ATTACK_RANGE`: Attack range
- `HEARTBEAT_INTERVAL`: Heartbeat interval
- `HEARTBEAT_TIMEOUT`: Heartbeat timeout

## Go Version (TUI)

### Compilation and Execution

```bash
cd warzone/go
go build ./cmd/server
go build ./cmd/client
```

### Starting the Server

```bash
./server
# Default listens on :8888
```

### Starting the Client

```bash
./client
# After startup, enter server address and username
```

### Controls

- **Arrow Keys**: Move
- **Tab**: Select opponent to challenge
- **Enter**: Confirm challenge
- **Esc**: Exit

## Technical Highlights

### C++ Version

- Implements network communication using raw sockets
- Uses SQLite for data storage
- Heartbeat mechanism to detect player online status
- Mutex locks to protect shared game state

### Go Version

- Uses TUI library for terminal interface
- JSON-based communication protocol
- SQLite for storing player data
- Heartbeat monitoring with automatic disconnection handling

## License

This project is intended solely for learning and research purposes.