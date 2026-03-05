<p align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/Platform-macOS%20%7C%20Linux-lightgrey?style=for-the-badge" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" />
  <img src="https://img.shields.io/badge/Status-Active-brightgreen?style=for-the-badge" />
</p>

<h1 align="center">claude-connector</h1>

<p align="center">
  <b>LAN-based P2P Claude.ai session sharing proxy</b><br/>
  Share your team's Claude capacity transparently — no rate limits, no waiting.
</p>

---

## What Is This?

When you're working in a small team with Claude.ai Pro/Team subscriptions, each account hits rate limits independently. One person gets blocked while the rest of the team is under capacity — wasted quota, interrupted flow.

**claude-connector** solves this by running a local proxy on every machine. It automatically:

1. Tries each of your local session tokens in turn — failover is transparent, the client never sees a 429
2. Routes to a teammate's machine when all your local sessions are rate-limited
3. Falls back to Ollama or LM Studio when the whole team is busy
4. Returns a `429` only when **everyone** is out of capacity

Because the Anthropic API is stateless (every request already carries the full conversation history), retrying with a different session token requires no special buffering — the exact same request body is forwarded immediately.

Every machine on the LAN discovers each other automatically via **mDNS** — zero configuration, no central server.

---

## How It Works

```
Your App / Claude CLI
        │
        ▼
 ┌─────────────────┐
 │  Proxy  :8765   │  ← Anthropic-compatible API
 └────────┬────────┘
          │
    ┌─────▼──────────────────────────────────┐
    │           Routing Logic                │
    │                                        │
    │  1. Try each local session     ──► Failover on 429
    │  2. Peer available on LAN?    ──► Forward
    │  3. Ollama / LM Studio up?    ──► Fallback
    │  4. Nothing available?        ──► 429
    └────────────────────────────────────────┘
          │
    ┌─────▼──────┐    ┌─────────────┐    ┌──────────────┐
    │  Sessions  │    │  LAN Peers  │    │   Fallback   │
    │  Pool      │    │  (mDNS P2P) │    │  Ollama/LMS  │
    └────────────┘    └─────────────┘    └──────────────┘
```

> **Security note:** Session tokens never leave the machine that owns them. Peers forward only the request *content* and return the response — your credentials stay local.

---

## Features

| Feature | Details |
|---|---|
| **Anthropic-compatible proxy** | Drop-in replacement — point any client to `:8765` |
| **Automatic LAN discovery** | mDNS (`_claude-connector._tcp`) — peers appear within ~5s |
| **Smart routing** | Local (all sessions) → Peer → Ollama/LM Studio → 429 |
| **Transparent multi-session failover** | When one local session hits a 429, the proxy instantly retries with the next available session — the client never sees the error |
| **Rate-limit awareness** | Detects `429` responses, backs off exponentially (60s → 120s → 240s → 600s) |
| **Two session types** | `anthropic_api` (sk-ant-... key) and `claude_web` (session cookie) |
| **TUI dashboard** | Real-time Bubble Tea terminal UI with ASCII network graph; failover retries shown as `(+N retry)` in the request log |
| **Web dashboard** | D3.js force graph at `localhost:8766` with live WebSocket updates |
| **Gossip protocol** | Peers share status every 5s so routing decisions are always fresh |
| **HMAC-SHA256 auth** | All peer-to-peer traffic is signed — replay attacks blocked |
| **Fallback backends** | Auto-detects Ollama (`:11434`) and LM Studio (`:1234`) |
| **No Claude CLI required** | Standalone binary — works with any Anthropic-compatible client |
| **Single binary** | No runtime dependencies — just copy and run |

---

## Quick Start

### 1. Build

```bash
# Clone or copy the project, then:
bash setup.sh
```

`setup.sh` will:
- Install Go 1.23 automatically if not present (macOS + Linux)
- Run `go mod tidy` to fetch all dependencies
- Build the `claude-connector` binary
- Create a default config at `~/.config/claude-connector/config.toml`

### 2. Add Your Session Key

Open `~/.config/claude-connector/config.toml` and add your Claude credentials:

```toml
# Option A — Official Anthropic API key
[[sessions.session]]
id          = "personal"
type        = "anthropic_api"
session_key = "sk-ant-api03-..."
enabled     = true

# Option B — Claude.ai web session cookie
[[sessions.session]]
id          = "web-session"
type        = "claude_web"
session_key = "sk-ant-sid01-..."
enabled     = true
```

### 3. Start

```bash
# Interactive TUI (recommended)
./claude-connector start

# Headless — no terminal UI
./claude-connector start --no-tui

# Web dashboard only
./claude-connector start --web
```

### 4. Point Your Client Here

```bash
# Claude CLI
claude --api-url http://localhost:8765

# curl
curl http://localhost:8765/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: any-value" \
  -d '{"model":"claude-3-5-sonnet-20241022","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}'

# Python (Anthropic SDK)
import anthropic
client = anthropic.Anthropic(base_url="http://localhost:8765", api_key="any-value")
```

---

## TUI Dashboard

```
┌─ claude-connector v1.0  node:alice-mbp  proxy::8765  uptime:2h14m ──────────┐
│ SESSIONS                    │ PEER NETWORK                                    │
│ ● personal     AVAILABLE    │       ┌──────┐                                 │
│ ● team-shared  COOLING 44s  │       │alice │◄───┐                            │
│ ● api-backup   AVAILABLE    │       └──┬───┘    │                            │
│                             │     ┌───▼──┐  ┌──┴───┐                        │
│ FALLBACK                    │     │ bob  │  │carol │                         │
│ ● Ollama  :11434  ✓         │     │2 av  │  │0 av  │                        │
│ ○ LMStudio :1234  ✗         │     └──────┘  └──────┘                        │
│                             ├────────────────────────────────────────────────┤
│                             │ REQUEST LOG                                     │
│                             │ 14:32 → local        200  1.2s  claude-3-5    │
│                             │ 14:33 → local (+1 retry) 200 2.1s claude-3-5  │
│                             │ 14:34 → peer:bob     200  3.4s  claude-3-5    │
│                             │ 14:35 → ollama       200  1.8s  llama3.2      │
├─────────────────────────────┴────────────────────────────────────────────────┤
│ [q]uit  [a]dd-session  [d]isable  [w]eb-dashboard  [?]help                  │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Keyboard shortcuts:**

| Key | Action |
|-----|--------|
| `q` | Quit |
| `a` | Add session |
| `d` | Disable selected session |
| `w` | Open web dashboard in browser |
| `?` | Help |

---

## Web Dashboard

Open `http://localhost:8766` in your browser for a live D3.js network graph:

- **Nodes** = peers on the LAN, colored by capacity (green / amber / red)
- **Animated edges** = active request routing between peers
- **Real-time** via WebSocket — updates as sessions change state
- **Settings panel** — add/remove sessions, view logs

---

## Configuration Reference

Config file: `~/.config/claude-connector/config.toml`

```toml
# Unique identifier for this node (auto-generated)
node_id   = "550e8400-e29b-41d4-a716-446655440000"
node_name = "alice-mbp"

# ── Proxy ────────────────────────────────────────────────────────────────────
[proxy]
port    = 8765   # Anthropic-compatible API endpoint
api_key = ""     # Optional: require clients to send this key

# ── Peer API ─────────────────────────────────────────────────────────────────
[peer]
port          = 8767
shared_secret = "generate-with: openssl rand -hex 32"
# All machines on the team must use the same shared_secret

# ── Web Dashboard ─────────────────────────────────────────────────────────────
[web]
port         = 8766
bind_address = "127.0.0.1"   # Change to "0.0.0.0" to expose on LAN

# ── Fallback Backends ─────────────────────────────────────────────────────────
[fallback]
ollama_enabled        = true
ollama_url            = "http://localhost:11434"
ollama_default_model  = "llama3.2:latest"
lmstudio_enabled      = true
lmstudio_url          = "http://localhost:1234"

# ── Sessions ──────────────────────────────────────────────────────────────────
# Add multiple sessions for automatic failover.
# When session-1 hits a rate limit, the proxy immediately retries with session-2
# (and so on) before falling through to peers or fallback backends.

[[sessions.session]]
id          = "personal"
type        = "anthropic_api"    # or "claude_web"
session_key = "sk-ant-..."
enabled     = true

[[sessions.session]]
id          = "team-account"
type        = "claude_web"
session_key = "sk-ant-sid01-..."
enabled     = true
```

### Session Types

| Type | Key Format | Description |
|------|-----------|-------------|
| `anthropic_api` | `sk-ant-api03-...` | Official Anthropic API key — most reliable |
| `claude_web` | `sk-ant-sid01-...` | Claude.ai browser session cookie |

To get a `claude_web` session key:
1. Log into [claude.ai](https://claude.ai) in your browser
2. Open DevTools → Application → Cookies
3. Copy the value of `sessionKey`

---

## Multi-Machine Setup (Team)

Every team member follows the same steps. The **only shared value** is `shared_secret` in `[peer]` — it must be identical on all machines.

```
Machine A (alice)          Machine B (bob)            Machine C (carol)
─────────────────          ───────────────            ─────────────────
./claude-connector start   ./claude-connector start   ./claude-connector start
        │                          │                          │
        └──────── mDNS discovery ──┴──────── mDNS discovery ─┘
                     (automatic, ~5 seconds)
```

**To generate a shared secret:**
```bash
openssl rand -hex 32
```

Add the same output to all machines' `config.toml` under `[peer] shared_secret`.

---

## Session State Machine

Each session goes through these states automatically:

```
                    ┌──────────────┐
         ┌─acquire──►   IN_USE     ├──success──┐
         │          └──────┬───────┘           │
         │                 │ 429               │
         │                 ▼                   ▼
  ┌──────┴───────┐   ┌─────────────┐    ┌──────────────┐
  │  AVAILABLE   │◄──┤COOLING_DOWN │    │  AVAILABLE   │
  └──────────────┘   └─────────────┘    └──────────────┘
                       (exponential
                        backoff:
                        60→120→240→600s)
```

---

## Ports

| Port | Service | Description |
|------|---------|-------------|
| `8765` | Proxy API | Anthropic-compatible endpoint for clients |
| `8766` | Web Dashboard | Browser UI + REST API (`/api/status`, `/api/peers`, `/api/sessions`) |
| `8767` | Peer API | Internal P2P communication (HMAC-authenticated) |

---

## CLI Reference

```
claude-connector [command] [flags]
```

### Commands

| Command | Description |
|---------|-------------|
| `start` | Start the proxy and all services |
| `status` | Show status of a running instance |

### `start` Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--no-tui` | `false` | Run headless — no terminal UI |
| `--web` | `false` | Skip TUI, open web dashboard mode |
| `--config` | `~/.config/claude-connector/config.toml` | Path to config file |

### Examples

```bash
# Start with TUI
./claude-connector start

# Start headless (e.g. in a tmux session or systemd)
./claude-connector start --no-tui

# Use a custom config file
./claude-connector start --config /path/to/config.toml

# Check status of a running instance
./claude-connector status

# Stop the server
pkill -f "claude-connector start"
```

---

## REST API

When running, the web server exposes a simple REST API:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/status` | Node status, ports, uptime, session summary |
| `GET` | `/api/peers` | All discovered LAN peers |
| `GET` | `/api/sessions` | All session states |
| `POST` | `/api/sessions/add` | Add a new session at runtime |

**Example:**
```bash
curl http://localhost:8766/api/status | jq .
```

```json
{
  "node_id": "550e8400-...",
  "node_name": "alice-mbp",
  "uptime_seconds": 3720,
  "proxy_port": 8765,
  "peer_port": 8767,
  "web_port": 8766,
  "sessions": [...],
  "peers": [...],
  "fallbacks": [
    { "Name": "Ollama", "Available": true },
    { "Name": "LM Studio", "Available": false }
  ]
}
```

---

## Project Structure

```
claude-connector/
├── main.go
├── setup.sh                     # One-shot build & setup script
├── cmd/
│   ├── root.go                  # Cobra CLI root
│   ├── start.go                 # `start` command
│   └── status.go                # `status` command
└── internal/
    ├── config/
    │   ├── config.go            # TOML config structs + load/save
    │   └── validate.go          # Config validation
    ├── session/
    │   ├── session.go           # State machine (AVAILABLE → IN_USE → COOLING_DOWN)
    │   ├── pool.go              # Session pool: acquire/release
    │   ├── claude_client.go     # Claude.ai web API client
    │   ├── anthropic_client.go  # Official Anthropic API client
    │   └── ratelimit.go         # 429 detection + retry-after parsing
    ├── proxy/
    │   ├── server.go            # HTTP server on :8765
    │   ├── handler.go           # POST /v1/messages handler
    │   ├── router.go            # local → peer → fallback routing
    │   ├── translate.go         # Anthropic ↔ Claude.ai format translation
    │   └── stream.go            # SSE stream forwarding
    ├── peer/
    │   ├── discovery.go         # mDNS via zeroconf
    │   ├── registry.go          # Live peer list + latency tracking
    │   ├── server.go            # Peer API on :8767
    │   ├── client.go            # Forward requests to peers
    │   └── auth.go              # HMAC-SHA256 signing/verification
    ├── fallback/
    │   ├── detector.go          # Auto-detect Ollama / LM Studio
    │   ├── ollama.go            # Ollama client
    │   └── lmstudio.go          # LM Studio client
    ├── gossip/
    │   ├── engine.go            # Gossip rounds (5s interval, fan-out=3)
    │   └── protocol.go          # Message types
    ├── tui/
    │   ├── app.go               # Bubble Tea root model
    │   ├── update.go            # Key/event handling
    │   ├── view.go              # Layout rendering
    │   └── components/
    │       ├── sessions.go      # Session list panel
    │       ├── peers.go         # Peer list panel
    │       ├── network_graph.go # ASCII circular force graph
    │       └── request_log.go   # Request log panel
    └── web/
        ├── server.go            # Embedded HTTP + WebSocket server on :8766
        ├── handlers.go          # REST handlers
        ├── ws.go                # WebSocket broadcast hub
        ├── index.html           # Dashboard HTML
        ├── app.js               # D3.js force graph + WebSocket client
        ├── style.css            # Dashboard styles
        └── d3.v7.min.js         # Vendored D3.js (no CDN dependency)
```

---

## Security

- **Session tokens never leave the owning machine.** When your session forwards a request through a peer, it sends only the message content — the peer executes it using their own token and returns the response.
- **HMAC-SHA256** signs every peer-to-peer request. The signature includes method, path, timestamp, nonce, and body hash.
- **Replay protection**: requests are rejected outside a 30-second timestamp window, and nonces are tracked in an LRU cache.
- **Shared secret** is the only credential shared across the team. Rotate it by updating `config.toml` on all machines.
- **Web dashboard** binds to `127.0.0.1` by default — not reachable from the network.

---

## Requirements

| | Minimum |
|--|---------|
| **Go** | 1.21+ (auto-installed by `setup.sh`) |
| **OS** | macOS or Linux |
| **Network** | All machines on the same LAN (mDNS/multicast must work) |
| **Claude** | At least one session key (`anthropic_api` or `claude_web`) |

> `claude-connector` does **not** require Claude CLI, Node.js, Python, or any other runtime. It's a single static binary.

---

## Troubleshooting

**`./claude-connector status` says "not running"**
```bash
# Make sure the server is started first
./claude-connector start --no-tui &
./claude-connector status
```

**Peers not discovering each other**
- Ensure all machines are on the same LAN subnet
- Check that mDNS/multicast traffic is not blocked by a firewall
- Verify `shared_secret` is identical in all `config.toml` files
- Wait ~5 seconds after startup for mDNS to propagate

**Port already in use**
```bash
# Kill any lingering instances
pkill -f "claude-connector start"
# Then restart
./claude-connector start
```

**Requests returning 429 immediately**
- No session keys are configured — add one to `config.toml`
- All sessions are in `COOLING_DOWN` state — wait for backoff to expire (check TUI or `/api/sessions`). With multiple sessions configured, the proxy will automatically try each one before falling through to peers.
- Ollama/LM Studio fallback not running — start one or configure it

**TUI looks broken / garbled**
- Use a modern terminal emulator (iTerm2, Kitty, Ghostty, WezTerm)
- Minimum 120×30 terminal recommended

---

## License

MIT — see [LICENSE](LICENSE) for details.

---

<p align="center">
  Built with <a href="https://github.com/charmbracelet/bubbletea">Bubble Tea</a> · <a href="https://d3js.org">D3.js</a> · <a href="https://github.com/grandcat/zeroconf">zeroconf</a> · Go
</p>
