# Guardian AI

AI-powered DNS filtering server with machine learning threat detection, blocklist support, per-client rules, and a web dashboard.

---

## What It Does

Guardian AI sits between your devices and the internet as a DNS server. Every query is checked against blocklists, per-client rules, and a CNN-LSTM ML classifier before being forwarded upstream.

```
Device --> Guardian AI (DNS :53) --> Upstream DNS (8.8.8.8, 1.1.1.1, etc.)
                |
                |-- Global blocklist (hosts/AdGuard format)
                |-- Per-client rules (AdGuard filter syntax)
                |-- ML classifier (CNN-LSTM via gRPC)
```

---

## Requirements

- Go 1.24+
- Node.js 18+ and npm
- Python 3.9+ with pip

---

## Install

### 1. Train the ML model (first time only)

```bash
cd ml-service
pip install -r requirements.txt
python train_model.py
cd ..
```

This generates `guardian_model.h5` and `tokenizer.pickle` in `ml-service/`.

### 2. Build the frontend

```bash
cd frontend
npm install
npm run build
cd ..
```

### 3. Build the server

```bash
go build -o guardian-ai .
```

---

## Run

```bash
./guardian-ai
```

The server starts DNS on port 53, the web dashboard on port 8081, and automatically extracts and launches the embedded ML service.

On first run, open `http://localhost:8081` and complete the setup wizard to create your admin account.

Then point your router or device DNS to the machine running Guardian AI.

---

## Command-Line Flags

| Flag | Default | Description |
|---|---|---|
| `-listen` | `:53` | DNS listen address (UDP and TCP) |
| `-upstream` | `8.8.8.8:53 1.1.1.1:53` | Upstream DNS servers, space-separated |
| `-blocklist` | `blocklists/hosts.txt` | Path to persist the merged blocklist |
| `-ml` | `localhost:50051` | ML gRPC service address |
| `-db` | `guardian.db` | SQLite database path |
| `-web` | `:8081` | Web dashboard listen address |
| `-log-level` | `warn` | Log verbosity: `error`, `warn`, `info`, `debug` |
| `-verbose` | `false` | Shorthand for `-log-level info` |
| `-frontend-dev` | `false` | Enable CORS for Vite dev server at `localhost:5173` |

---

## Project Structure

```
guardian-ai/
  main.go               Go DNS server, HTTP API, embedded SPA
  go.mod
  proto/
    guardian.proto       gRPC service definition
    guardian.pb.go       Generated Go stubs
    guardian_grpc.pb.go
  frontend/             React + Vite + Tailwind dashboard
    src/
      pages/            Dashboard, Queries, Clients, Services, ML, Settings
      components/       Shared UI components
  ml-service/           Python gRPC ML service
    guardian_grpc.py     gRPC server
    train_model.py      Model training script
    create_dataset.py   Dataset generation
    requirements.txt
```

---

## How DNS Resolution Works

Queries are resolved in this order:

1. DNS cache hit -- return cached response
2. Rate limit exceeded -- REFUSED
3. Per-client block rule matches -- NXDOMAIN
4. Per-client allow rule matches -- skip to step 8
5. Client entirely blocked -- NXDOMAIN
6. Global blocklist match -- NXDOMAIN
7. ML classification (A/AAAA queries only) -- block or pass
8. Forward to upstream resolvers (parallel, fastest wins)

---

## License

MIT