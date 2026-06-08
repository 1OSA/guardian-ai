# Guardian AI

AI-powered DNS filtering server with machine learning threat detection, blocklist support, and a web dashboard.

## What It Does

Guardian AI sits between your devices and the internet as a DNS server. Every query is checked against blocklists, per-client rules, and an ML classifier before forwarding to upstream DNS.

## Requirements

- Go 1.24+
- Node.js 18+ and npm
- Python 3.9+ with pip

## Quick Start

### 1. Build the Frontend

```bash
cd frontend
npm install && npm run build
cd ..
```

### 2. Build the Server

```bash
go build -o guardian-ai .
```

### 3. Run

```bash
./guardian-ai
```

The server will start:
- DNS server on port 53
- Web dashboard on port 8081
- Embedded ML service (gRPC on localhost:50051)

Open `http://localhost:8081` and complete the setup wizard.

## Command-Line Flags

| Flag | Default | Description |
|---|---|---|
| `-listen` | `:53` | DNS listen address |
| `-upstream` | `8.8.8.8:53 1.1.1.1:53` | Upstream DNS servers |
| `-ml` | `localhost:50051` | ML gRPC service address |
| `-db` | `guardian.db` | SQLite database path |
| `-web` | `:8081` | Web dashboard address |
| `-log-level` | `warn` | Log verbosity: error, warn, info, debug |
| `-verbose` | `false` | Enable info-level logging |

## Project Structure

```
guardian-ai/
  main.go              DNS server entry point
  internal/
    server/app.go      GuardianServer (DNS, HTTP API, database)
    cache/cache.go     LRU caches and rate limiting
    config/config.go   Configuration and constants
  frontend/            React dashboard
  proto/               gRPC service definitions
  ml-service/          Python ML service
```

## Features

- Global and per-client blocklists
- Per-client/group filtering rules
- Service-based blocking (Netflix, YouTube, Discord, etc.)
- ML threat detection with configurable threshold
- Rate limiting per client
- User authentication and dashboard
- SQLite database for persistence
- Query logging and statistics

## Screenshots

![Clients-Guardian AI](REPORT_assets/Clients-Guardian%20AI.png)
![DASHBOARD-Guardian AI](REPORT_assets/DASHBOARD-Guardian%20AI.png)
![Query-log-Guardian AI](REPORT_assets/Query-log-Guardian%20AI.png)
![Services-Guardian AI](REPORT_assets/Services-Guardian%20AI.png)
![Settings-Guardian AI](REPORT_assets/Settings-Guardian%20AI.png)
![Threat Detection-Guardian AI](REPORT_assets/Threat%20Detection-Guardian%20AI.png)

## License

MIT
