#!/usr/bin/env bash
# Migrate one host's Komari Server to Komari Probe. Agent hosts use the Agent repo script.
set -u -o pipefail

REPO="komari-probe/komari-probe"; TAG="latest"
INSTALL_DIR="/opt/komari"; DATA_DIR="/opt/komari/data"; SERVICE="komari"; PORT="25774"
BACKUP_ROOT="/var/backups/komari-server-migration"; BINARY_URL=""; CHECKSUM_URL=""; CLEANUP_ID=""; DRY_RUN=0
TMP_DIR=""; BACKUP_DIR=""

usage() { cat <<'EOF'
Usage: sudo bash migrate-server-host.sh [options]

Backs up this host's old Server binary and complete data directory, installs a
SHA-256-verified Komari Probe Server, verifies it locally, and rolls back both
binary and data if replacement fails. It does not touch any Agent.

Options:
  --install-dir PATH       Default: /opt/komari
  --data-dir PATH          Default: /opt/komari/data
  --service NAME           Default: komari
  --port PORT              Local health-check port; default: 25774
  --backup-root PATH       Default: /var/backups/komari-server-migration
  --tag TAG                Default: latest stable release
  --url URL                Override binary URL (controlled testing)
  --checksum-url URL       Override checksums.txt URL
  --dry-run                Print the plan without changing this host
  --cleanup-backup ID      Explicitly delete one completed backup
  -h, --help               Show this help
EOF
}
log() { printf '[komari-server-migrate] %s\n' "$*"; }
die() { printf '[komari-server-migrate] ERROR: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"; }
arch() { case "$(uname -m)" in x86_64|amd64) echo amd64;; aarch64|arm64) echo arm64;; i386|i686) echo 386;; riscv64) echo riscv64;; loongarch64|loong64) echo loong64;; *) die "Unsupported CPU architecture: $(uname -m)";; esac; }
sha256() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi; }
release_base() { [ "$TAG" = latest ] && printf 'https://github.com/%s/releases/latest/download' "$REPO" || printf 'https://github.com/%s/releases/download/%s' "$REPO" "$TAG"; }
backup_unit() { local fragment; mkdir -p "$BACKUP_DIR/systemd" || return 1; systemctl cat "$SERVICE" > "$BACKUP_DIR/systemd/$SERVICE.service.rendered" 2>/dev/null || true; fragment=$(systemctl show -p FragmentPath --value "$SERVICE" 2>/dev/null || true); [ -n "$fragment" ] && [ -f "$fragment" ] && cp -a "$fragment" "$BACKUP_DIR/systemd/$(basename "$fragment")"; return 0; }
backup() { BACKUP_DIR="$BACKUP_ROOT/$BACKUP_ID"; mkdir -p "$BACKUP_DIR" || return 1; cp -a "$INSTALL_DIR/komari" "$BACKUP_DIR/komari.old" || return 1; backup_unit || return 1; log "Archiving complete data directory: $DATA_DIR"; tar -C "$DATA_DIR" -czf "$BACKUP_DIR/data.tar.gz" . || return 1; sha256 "$BACKUP_DIR/data.tar.gz" > "$BACKUP_DIR/data.tar.gz.sha256" || return 1; }
restore() { [ -f "$BACKUP_DIR/komari.old" ] || return 0; log "Rolling back Server binary and data..."; systemctl stop "$SERVICE" >/dev/null 2>&1 || true; install -m 0755 "$BACKUP_DIR/komari.old" "$INSTALL_DIR/komari" || true; rm -rf "$DATA_DIR"; mkdir -p "$DATA_DIR"; tar -C "$DATA_DIR" -xzf "$BACKUP_DIR/data.tar.gz" || true; systemctl daemon-reload; systemctl start "$SERVICE" || log "Rollback could not restart $SERVICE; inspect systemctl status."; }
abort() { log "Migration failed: $1"; restore; exit 1; }
cleanup() { local target="$BACKUP_ROOT/$CLEANUP_ID" root target_real; [[ "$CLEANUP_ID" =~ ^[0-9]{8}T[0-9]{6}Z$ ]] || die "Backup ID must look like 20260922T120000Z."; [ -d "$target" ] || die "Backup does not exist: $target"; root=$(realpath -m "$BACKUP_ROOT"); target_real=$(realpath -m "$target"); [[ "$target_real" == "$root"/* ]] || die "Refusing to delete outside backup root."; rm -rf -- "$target_real"; log "Deleted explicitly selected backup: $target_real"; }
download() { local manifest="$TMP_DIR/checksums.txt" expected actual; curl -fsSL --connect-timeout 15 --retry 3 -o "$manifest" "$CHECKSUM_URL" || return 1; expected=$(awk -v n="$ASSET" '$2 == n || $2 == "*" n { print $1 }' "$manifest"); [ "$(printf '%s\n' "$expected" | sed '/^$/d' | wc -l | tr -d ' ')" = 1 ] || return 1; curl -fsSL --connect-timeout 15 --retry 3 -o "$TMP_DIR/$ASSET" "$BINARY_URL" || return 1; actual=$(sha256 "$TMP_DIR/$ASSET"); [ "$actual" = "$expected" ] || return 1; chmod 0755 "$TMP_DIR/$ASSET"; }

while [ "$#" -gt 0 ]; do case "$1" in --install-dir) INSTALL_DIR=$2; shift;; --data-dir) DATA_DIR=$2; shift;; --service) SERVICE=$2; shift;; --port) PORT=$2; shift;; --backup-root) BACKUP_ROOT=$2; shift;; --tag) TAG=$2; shift;; --url) BINARY_URL=$2; shift;; --checksum-url) CHECKSUM_URL=$2; shift;; --dry-run) DRY_RUN=1;; --cleanup-backup) CLEANUP_ID=$2; shift;; -h|--help) usage; exit 0;; *) die "Unknown option: $1";; esac; shift; done
[ "${EUID:-$(id -u)}" -eq 0 ] || die "Run as root (sudo)."; need curl; need tar; need systemctl; need realpath; need awk; command -v sha256sum >/dev/null 2>&1 || need shasum
[ -n "$CLEANUP_ID" ] && { cleanup; exit 0; }
ASSET="komari-linux-$(arch)"; BASE=$(release_base); BINARY_URL=${BINARY_URL:-"$BASE/$ASSET"}; CHECKSUM_URL=${CHECKSUM_URL:-"$BASE/checksums.txt"}; BACKUP_ID=$(date -u +%Y%m%dT%H%M%SZ)
[ -x "$INSTALL_DIR/komari" ] || die "Old Server binary not found: $INSTALL_DIR/komari"; [ -d "$DATA_DIR" ] || die "Data directory not found: $DATA_DIR"; systemctl status "$SERVICE" >/dev/null 2>&1 || die "Server service not found: $SERVICE"
log "Plan: Server $SERVICE; backup=$BACKUP_ROOT/$BACKUP_ID; asset=$ASSET"; [ "$DRY_RUN" -eq 0 ] || { log "Dry run finished; no changes were made."; exit 0; }
TMP_DIR=$(mktemp -d); trap 'rm -rf "$TMP_DIR"' EXIT
backup || die "Backup failed; old Server remains untouched."; download || die "Download or checksum verification failed; old Server remains untouched."
systemctl stop "$SERVICE" || abort "could not stop old Server"; install -m 0755 "$TMP_DIR/$ASSET" "$INSTALL_DIR/komari" || abort "could not install new binary"; systemctl start "$SERVICE" || abort "new Server did not start"; sleep 2; systemctl is-active --quiet "$SERVICE" || abort "new Server is not active"; curl -fsS --max-time 10 "http://127.0.0.1:$PORT/" >/dev/null || abort "local health endpoint did not respond"
log "Server migration verified. Backup retained at $BACKUP_DIR"
