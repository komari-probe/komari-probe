#!/usr/bin/env bash
# Migrate one host's Server to Sonar. Agent hosts use the Agent repo script.
set -u -o pipefail

REPO="sonar-probe/sonar"; TAG="latest"
INSTALL_DIR=""; DATA_DIR=""; SERVICE="sonar"; PORT="25774"; BINARY=""
BACKUP_ROOT="/var/backups/sonar-server-migration"; BINARY_URL=""; CHECKSUM_URL=""; CLEANUP_ID=""; DRY_RUN=0
TMP_DIR=""; BACKUP_DIR=""

usage() { cat <<'EOF'
Usage: sudo bash migrate-server-host.sh [options]

Backs up this host's old Server binary and complete data directory, installs a
SHA-256-verified Sonar Server, verifies it locally, and rolls back both
binary and data if replacement fails. It does not touch any Agent.

Options:
  --install-dir PATH       Server directory (auto-detected if omitted)
  --binary PATH            Server binary to replace (auto-detected if omitted)
  --data-dir PATH          Data directory (default: <install-dir>/data)
  --service NAME           Default: sonar (auto-falls back to komari)
  --port PORT              Local health-check port; default: 25774
  --backup-root PATH       Default: /var/backups/sonar-server-migration
  --tag TAG                Default: latest stable release
  --url URL                Override binary URL (controlled testing)
  --checksum-url URL       Override checksums.txt URL
  --dry-run                Print the plan without changing this host
  --cleanup-backup ID      Explicitly delete one completed backup
  -h, --help               Show this help
EOF
}
log() { printf '[sonar-server-migrate] %s\n' "$*"; }
die() { printf '[sonar-server-migrate] ERROR: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"; }
arch() { case "$(uname -m)" in x86_64|amd64) echo amd64;; aarch64|arm64) echo arm64;; i386|i686) echo 386;; riscv64) echo riscv64;; loongarch64|loong64) echo loong64;; *) die "Unsupported CPU architecture: $(uname -m)";; esac; }
sha256() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi; }
release_base() { [ "$TAG" = latest ] && printf 'https://github.com/%s/releases/latest/download' "$REPO" || printf 'https://github.com/%s/releases/download/%s' "$REPO" "$TAG"; }
detect_binary() { local p; for p in /opt/sonar/sonar /opt/komari/sonar /opt/komari/komari /usr/local/bin/sonar /usr/local/bin/komari; do [ -x "$p" ] && { printf '%s' "$p"; return; }; done; return 1; }
backup_unit() { local fragment; mkdir -p "$BACKUP_DIR/systemd" || return 1; systemctl cat "$SERVICE" > "$BACKUP_DIR/systemd/$SERVICE.service.rendered" 2>/dev/null || true; fragment=$(systemctl show -p FragmentPath --value "$SERVICE" 2>/dev/null || true); [ -n "$fragment" ] && [ -f "$fragment" ] && cp -a "$fragment" "$BACKUP_DIR/systemd/$(basename "$fragment")"; return 0; }
backup() { BACKUP_DIR="$BACKUP_ROOT/$BACKUP_ID"; mkdir -p "$BACKUP_DIR" || return 1; cp -a "$BINARY" "$BACKUP_DIR/$(basename "$BINARY").old" || return 1; backup_unit || return 1; log "Archiving complete data directory: $DATA_DIR"; tar -C "$DATA_DIR" -czf "$BACKUP_DIR/data.tar.gz" . || return 1; sha256 "$BACKUP_DIR/data.tar.gz" > "$BACKUP_DIR/data.tar.gz.sha256" || return 1; }
restore() { local old="$BACKUP_DIR/$(basename "$BINARY").old"; [ -f "$old" ] || return 0; log "Rolling back Server binary and data..."; systemctl stop "$SERVICE" >/dev/null 2>&1 || true; install -m 0755 "$old" "$BINARY" || true; rm -rf "$DATA_DIR"; mkdir -p "$DATA_DIR"; tar -C "$DATA_DIR" -xzf "$BACKUP_DIR/data.tar.gz" || true; systemctl daemon-reload; systemctl start "$SERVICE" || log "Rollback could not restart $SERVICE; inspect systemctl status."; }
abort() { log "Migration failed: $1"; restore; exit 1; }
cleanup() { local target="$BACKUP_ROOT/$CLEANUP_ID" root target_real; [[ "$CLEANUP_ID" =~ ^[0-9]{8}T[0-9]{6}Z$ ]] || die "Backup ID must look like 20260922T120000Z."; [ -d "$target" ] || die "Backup does not exist: $target"; root=$(realpath -m "$BACKUP_ROOT"); target_real=$(realpath -m "$target"); [[ "$target_real" == "$root"/* ]] || die "Refusing to delete outside backup root."; rm -rf -- "$target_real"; log "Deleted explicitly selected backup: $target_real"; }
download() { local manifest="$TMP_DIR/checksums.txt" expected actual; curl -fsSL --connect-timeout 15 --retry 3 -o "$manifest" "$CHECKSUM_URL" || return 1; expected=$(awk -v n="$ASSET" '$2 == n || $2 == "*" n { print $1 }' "$manifest"); [ "$(printf '%s\n' "$expected" | sed '/^$/d' | wc -l | tr -d ' ')" = 1 ] || return 1; curl -fsSL --connect-timeout 15 --retry 3 -o "$TMP_DIR/$ASSET" "$BINARY_URL" || return 1; actual=$(sha256 "$TMP_DIR/$ASSET"); [ "$actual" = "$expected" ] || return 1; chmod 0755 "$TMP_DIR/$ASSET"; }

while [ "$#" -gt 0 ]; do case "$1" in --install-dir) INSTALL_DIR=$2; shift;; --binary) BINARY=$2; shift;; --data-dir) DATA_DIR=$2; shift;; --service) SERVICE=$2; shift;; --port) PORT=$2; shift;; --backup-root) BACKUP_ROOT=$2; shift;; --tag) TAG=$2; shift;; --url) BINARY_URL=$2; shift;; --checksum-url) CHECKSUM_URL=$2; shift;; --dry-run) DRY_RUN=1;; --cleanup-backup) CLEANUP_ID=$2; shift;; -h|--help) usage; exit 0;; *) die "Unknown option: $1";; esac; shift; done
[ "${EUID:-$(id -u)}" -eq 0 ] || die "Run as root (sudo)."; need curl; need tar; need systemctl; need realpath; need awk; command -v sha256sum >/dev/null 2>&1 || need shasum
[ -n "$CLEANUP_ID" ] && { cleanup; exit 0; }
BINARY=${BINARY:-$(detect_binary || true)}; [ -n "$BINARY" ] || die "Could not detect old Server binary; pass --binary PATH."; [ -x "$BINARY" ] || die "Old Server binary not found: $BINARY"
INSTALL_DIR=${INSTALL_DIR:-"$(dirname "$BINARY")"}
DATA_DIR=${DATA_DIR:-"$INSTALL_DIR/data"}
[ -d "$DATA_DIR" ] || die "Data directory not found: $DATA_DIR"
if ! systemctl status "$SERVICE" >/dev/null 2>&1 && systemctl status "komari" >/dev/null 2>&1; then SERVICE="komari"; fi
systemctl status "$SERVICE" >/dev/null 2>&1 || die "Server service not found: $SERVICE"
ASSET="sonar-linux-$(arch)"; BASE=$(release_base); BINARY_URL=${BINARY_URL:-"$BASE/$ASSET"}; CHECKSUM_URL=${CHECKSUM_URL:-"$BASE/checksums.txt"}; BACKUP_ID=$(date -u +%Y%m%dT%H%M%SZ)
log "Plan: Server $SERVICE ($BINARY); backup=$BACKUP_ROOT/$BACKUP_ID; asset=$ASSET"; [ "$DRY_RUN" -eq 0 ] || { log "Dry run finished; no changes were made."; exit 0; }
TMP_DIR=$(mktemp -d); trap 'rm -rf "$TMP_DIR"' EXIT
backup || die "Backup failed; old Server remains untouched."; download || die "Download or checksum verification failed; old Server remains untouched."
systemctl stop "$SERVICE" || abort "could not stop old Server"
install -m 0755 "$TMP_DIR/$ASSET" "$BINARY" || abort "could not install new binary"
if [ "$(basename "$BINARY")" = "komari" ] && [ ! -e "$INSTALL_DIR/sonar" ]; then
    ln -sf "$BINARY" "$INSTALL_DIR/sonar" || true
fi
systemctl start "$SERVICE" || abort "new Server did not start"
sleep 2; systemctl is-active --quiet "$SERVICE" || abort "new Server is not active"
curl -fsS --max-time 10 "http://127.0.0.1:$PORT/" >/dev/null || abort "local health endpoint did not respond"
log "Server migration verified. Backup retained at $BACKUP_DIR"
