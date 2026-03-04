#!/usr/bin/env bash
# setup.sh — One-shot setup & build for claude-connector
# Usage: bash setup.sh
set -e

# ─── Colors ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
success() { echo -e "${GREEN}[OK]${NC}   $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
error()   { echo -e "${RED}[ERR]${NC}  $*" >&2; }
die()     { error "$*"; exit 1; }

echo -e "${BOLD}"
echo "╔══════════════════════════════════════════╗"
echo "║       claude-connector  setup            ║"
echo "╚══════════════════════════════════════════╝"
echo -e "${NC}"

# ─── Step 1: Find or install Go ───────────────────────────────────────────────
GO_MIN_MAJOR=1
GO_MIN_MINOR=21

find_go() {
  for candidate in \
    "$(command -v go 2>/dev/null)" \
    /usr/local/go/bin/go \
    /opt/homebrew/bin/go \
    "$HOME/go/bin/go" \
    "$HOME/.local/go/bin/go"
  do
    [[ -x "$candidate" ]] && echo "$candidate" && return 0
  done
  return 1
}

install_go_macos() {
  GO_VERSION="1.23.4"
  ARCH=$(uname -m)
  if [[ "$ARCH" == "arm64" ]]; then
    PKG="go${GO_VERSION}.darwin-arm64.pkg"
  else
    PKG="go${GO_VERSION}.darwin-amd64.pkg"
  fi
  URL="https://go.dev/dl/${PKG}"

  info "Downloading Go ${GO_VERSION} from ${URL} ..."
  TMP=$(mktemp -d)
  curl -fsSL -o "${TMP}/${PKG}" "${URL}" || die "Download failed"
  info "Installing (requires sudo) ..."
  sudo installer -pkg "${TMP}/${PKG}" -target / || die "Install failed"
  export PATH="/usr/local/go/bin:$PATH"
  rm -rf "${TMP}"
}

install_go_linux() {
  GO_VERSION="1.23.4"
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64)  GOARCH="amd64" ;;
    aarch64) GOARCH="arm64" ;;
    *)        die "Unsupported architecture: $ARCH" ;;
  esac
  TARBALL="go${GO_VERSION}.linux-${GOARCH}.tar.gz"
  URL="https://go.dev/dl/${TARBALL}"

  info "Downloading Go ${GO_VERSION} ..."
  TMP=$(mktemp -d)
  curl -fsSL -o "${TMP}/${TARBALL}" "${URL}" || die "Download failed"
  info "Extracting to /usr/local (requires sudo) ..."
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "${TMP}/${TARBALL}" || die "Extract failed"
  export PATH="/usr/local/go/bin:$PATH"
  rm -rf "${TMP}"
  # Persist in shell profile
  PROFILE=""
  [[ -f "$HOME/.zshrc" ]]  && PROFILE="$HOME/.zshrc"
  [[ -f "$HOME/.bashrc" ]] && PROFILE="$HOME/.bashrc"
  if [[ -n "$PROFILE" ]]; then
    grep -q '/usr/local/go/bin' "$PROFILE" || \
      echo 'export PATH="/usr/local/go/bin:$PATH"' >> "$PROFILE"
  fi
}

GO_BIN=$(find_go || true)

if [[ -z "$GO_BIN" ]]; then
  warn "Go not found. Installing automatically..."
  OS=$(uname -s)
  case "$OS" in
    Darwin) install_go_macos ;;
    Linux)  install_go_linux ;;
    *)      die "Cannot auto-install Go on $OS. Install it manually: https://go.dev/dl/" ;;
  esac
  GO_BIN=$(find_go) || die "Go install succeeded but binary not found. Open a new terminal and run: go mod tidy && go build -o claude-connector ."
fi

# Verify version
GO_VERSION_RAW=$("$GO_BIN" version 2>&1 | awk '{print $3}' | sed 's/go//')
GO_MAJOR=$(echo "$GO_VERSION_RAW" | cut -d. -f1)
GO_MINOR=$(echo "$GO_VERSION_RAW" | cut -d. -f2)

if [[ "$GO_MAJOR" -lt "$GO_MIN_MAJOR" ]] || \
   [[ "$GO_MAJOR" -eq "$GO_MIN_MAJOR" && "$GO_MINOR" -lt "$GO_MIN_MINOR" ]]; then
  die "Go ${GO_VERSION_RAW} is too old. Need >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR}. Upgrade at https://go.dev/dl/"
fi

success "Go ${GO_VERSION_RAW} found at ${GO_BIN}"
export PATH="$(dirname "$GO_BIN"):$PATH"

# ─── Step 2: cd into project directory ────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
info "Working directory: $SCRIPT_DIR"

# ─── Step 3: go mod tidy ──────────────────────────────────────────────────────
info "Running: go mod tidy  (downloads all dependencies) ..."
go mod tidy || die "go mod tidy failed"
success "Dependencies resolved"

# ─── Step 4: go build ─────────────────────────────────────────────────────────
info "Building claude-connector binary ..."
go build -ldflags="-s -w" -o claude-connector . || die "Build failed"
success "Binary built: ${SCRIPT_DIR}/claude-connector"

# ─── Step 5: Create default config if missing ─────────────────────────────────
CONFIG_DIR="$HOME/.config/claude-connector"
CONFIG_FILE="$CONFIG_DIR/config.toml"

if [[ ! -f "$CONFIG_FILE" ]]; then
  mkdir -p "$CONFIG_DIR"
  HOSTNAME=$(hostname -s 2>/dev/null || echo "my-machine")
  NODE_ID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "$(date +%s)-$$")

  cat > "$CONFIG_FILE" << TOML
node_id   = "${NODE_ID}"
node_name = "${HOSTNAME}"

[proxy]
port    = 8765
api_key = ""

[peer]
port          = 8767
shared_secret = "$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p | tr -d '\n')"

[web]
port         = 8766
bind_address = "127.0.0.1"

[fallback]
ollama_enabled        = true
ollama_url            = "http://localhost:11434"
ollama_default_model  = "llama3.2:latest"
lmstudio_enabled      = true
lmstudio_url          = "http://localhost:1234"

# Add your Claude session below.
# type = "anthropic_api"  → use an Anthropic API key (sk-ant-...)
# type = "claude_web"     → use a claude.ai session cookie (sk-ant-sid01-...)
#
# [[sessions.session]]
# id          = "personal"
# type        = "anthropic_api"
# session_key = "sk-ant-YOUR-KEY-HERE"
# enabled     = true
TOML

  success "Default config created: $CONFIG_FILE"
  warn "ACTION REQUIRED: Add at least one session key to $CONFIG_FILE before starting."
else
  info "Config already exists: $CONFIG_FILE"
fi

# ─── Step 6: Verify the binary works ──────────────────────────────────────────
info "Running self-test: ./claude-connector --help ..."
./claude-connector --help > /dev/null && success "Binary runs OK"

# ─── Done ─────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}${GREEN}Setup complete!${NC}"
echo ""
echo -e "  ${BOLD}1. Edit your config:${NC}"
echo -e "     ${YELLOW}$CONFIG_FILE${NC}"
echo -e "     Add your session key under [[sessions.session]]"
echo ""
echo -e "  ${BOLD}2. Start the proxy:${NC}"
echo -e "     ${GREEN}./claude-connector start${NC}             # TUI mode"
echo -e "     ${GREEN}./claude-connector start --no-tui${NC}    # headless"
echo -e "     ${GREEN}./claude-connector start --web${NC}       # skip TUI, open browser"
echo ""
echo -e "  ${BOLD}3. Point Claude CLI here:${NC}"
echo -e "     ${GREEN}claude --api-url http://localhost:8765${NC}"
echo ""
echo -e "  ${BOLD}4. Web dashboard:${NC}"
echo -e "     ${GREEN}http://localhost:8766${NC}"
echo ""
echo -e "  ${BOLD}5. Check status (while running):${NC}"
echo -e "     ${GREEN}./claude-connector status${NC}"
echo ""
