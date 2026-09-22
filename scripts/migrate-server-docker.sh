#!/usr/bin/env bash
# Docker Compose Server migration to Sonar. No Coolify dependency.
set -u -o pipefail

COMPOSE_FILE=""; SERVICE=""; TARGET_IMAGE=""; PROJECT=""; DATA_PATH="/app/data"; PORT="25774"
BACKUP_ROOT="/var/backups/sonar-server-docker-migration"; OVERRIDE_FILE=""; CLEANUP_ID=""; DRY_RUN=0
BACKUP_DIR=""; OVERRIDE_CREATED=0; MOUNT_TYPE=""; MOUNT_NAME=""; MOUNT_SOURCE=""

usage() { cat <<'EOF'
Usage: sudo bash migrate-server-docker.sh --compose-file FILE --service NAME --target-image IMAGE [options]

Migrates one Docker Compose Server service without requiring Coolify. It records
the old image/configuration, archives the persistent /app/data mount, creates a
small Compose image override, then recreates and verifies only that service.
If the replacement fails, it restores the data and recreates the old service.

Required:
  --compose-file FILE       Existing Docker Compose file
  --service NAME            Server service in that Compose project
  --target-image IMAGE      New Sonar image (prefer an immutable digest)

Options:
  --project NAME            Compose project name, if not inferred
  --data-path PATH          Container data mount; default: /app/data
  --port PORT               Container health port; default: 25774
  --backup-root PATH        Default: /var/backups/sonar-server-docker-migration
  --override-file FILE      Persistent image override file (default beside Compose file)
  --dry-run                 Validate and print the plan only
  --cleanup-backup ID       Explicitly delete one completed backup
EOF
}
log() { printf '[sonar-server-docker] %s\n' "$*"; }
die() { printf '[sonar-server-docker] ERROR: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"; }
compose() { if [ -n "$PROJECT" ]; then docker compose -p "$PROJECT" -f "$COMPOSE_FILE" "$@"; else docker compose -f "$COMPOSE_FILE" "$@"; fi; }
compose_target() { if [ -n "$PROJECT" ]; then docker compose -p "$PROJECT" -f "$COMPOSE_FILE" -f "$OVERRIDE_FILE" "$@"; else docker compose -f "$COMPOSE_FILE" -f "$OVERRIDE_FILE" "$@"; fi; }

cleanup_backup() { local target="$BACKUP_ROOT/$CLEANUP_ID" root resolved; [[ "$CLEANUP_ID" =~ ^[0-9]{8}T[0-9]{6}Z$ ]] || die "Backup ID must look like 20260922T120000Z."; [ -d "$target" ] || die "Backup does not exist: $target"; root=$(realpath -m "$BACKUP_ROOT"); resolved=$(realpath -m "$target"); [[ "$resolved" == "$root"/* ]] || die "Refusing to delete outside backup root."; rm -rf -- "$resolved"; log "Deleted explicitly selected backup: $resolved"; }
mount_for_path() { local cid=$1 target=$2 line type name source destination; while IFS='|' read -r type name source destination; do [[ "$target" == "$destination" ]] && { MOUNT_TYPE=$type; MOUNT_NAME=$name; MOUNT_SOURCE=$source; return 0; }; done < <(docker inspect --format '{{range .Mounts}}{{printf "%s|%s|%s|%s\n" .Type .Name .Source .Destination}}{{end}}' "$cid"); return 1; }
archive_mount() { case "$MOUNT_TYPE" in volume) docker run --rm -v "$MOUNT_NAME":/source:ro -v "$BACKUP_DIR":/backup alpine:3.21 tar -C /source -czf /backup/data.tar.gz .;; bind) tar -C "$MOUNT_SOURCE" -czf "$BACKUP_DIR/data.tar.gz" .;; *) return 1;; esac; }
restore_mount() { case "$MOUNT_TYPE" in volume) docker run --rm -v "$MOUNT_NAME":/target -v "$BACKUP_DIR":/backup alpine:3.21 sh -c 'rm -rf /target/* /target/.[!.]* /target/..?* 2>/dev/null || true; tar -C /target -xzf /backup/data.tar.gz';; bind) rm -rf "$MOUNT_SOURCE"/* "$MOUNT_SOURCE"/.[!.]* "$MOUNT_SOURCE"/..?* 2>/dev/null || true; tar -C "$MOUNT_SOURCE" -xzf "$BACKUP_DIR/data.tar.gz";; esac; }
rollback() { log "Rolling back Server image and persistent data..."; compose_target stop "$SERVICE" >/dev/null 2>&1 || true; restore_mount || log "Data restore failed; backup remains at $BACKUP_DIR"; [ "$OVERRIDE_CREATED" -eq 1 ] && rm -f -- "$OVERRIDE_FILE"; compose up -d --no-build "$SERVICE" || log "Could not recreate old service; inspect docker compose logs."; }
abort() { log "Migration failed: $1"; rollback; exit 1; }

while [ "$#" -gt 0 ]; do case "$1" in --compose-file) COMPOSE_FILE=$2; shift;; --service) SERVICE=$2; shift;; --target-image) TARGET_IMAGE=$2; shift;; --project) PROJECT=$2; shift;; --data-path) DATA_PATH=$2; shift;; --port) PORT=$2; shift;; --backup-root) BACKUP_ROOT=$2; shift;; --override-file) OVERRIDE_FILE=$2; shift;; --dry-run) DRY_RUN=1;; --cleanup-backup) CLEANUP_ID=$2; shift;; -h|--help) usage; exit 0;; *) die "Unknown option: $1";; esac; shift; done
[ "${EUID:-$(id -u)}" -eq 0 ] || die "Run as root (sudo)."; need docker; need tar; need realpath; need awk
[ -n "$CLEANUP_ID" ] && { cleanup_backup; exit 0; }
[ -n "$COMPOSE_FILE" ] && [ -f "$COMPOSE_FILE" ] || die "--compose-file must name an existing file."; [ -n "$SERVICE" ] || die "--service is required."; [ -n "$TARGET_IMAGE" ] || die "--target-image is required."
compose config --services | grep -Fx "$SERVICE" >/dev/null || die "Service not found in Compose file: $SERVICE"
CID=$(compose ps -q "$SERVICE"); [ -n "$CID" ] || die "Service is not running: $SERVICE"
mount_for_path "$CID" "$DATA_PATH" || die "No persistent mount at $DATA_PATH. Refusing to migrate container-local Server data."
BACKUP_ID=$(date -u +%Y%m%dT%H%M%SZ); BACKUP_DIR="$BACKUP_ROOT/$BACKUP_ID"; OVERRIDE_FILE=${OVERRIDE_FILE:-"$(dirname "$COMPOSE_FILE")/.sonar-${SERVICE}.override.yml"}
[ ! -e "$OVERRIDE_FILE" ] || die "Override already exists: $OVERRIDE_FILE. Use that Compose configuration or choose --override-file."
log "Plan: service=$SERVICE; old-image=$(docker inspect --format '{{.Config.Image}}' "$CID"); target=$TARGET_IMAGE; data=$MOUNT_TYPE:$MOUNT_SOURCE; backup=$BACKUP_DIR"
[ "$DRY_RUN" -eq 0 ] || { log "Dry run finished; no changes were made."; exit 0; }
mkdir -p "$BACKUP_DIR" || die "Cannot create backup directory."; cp -a "$COMPOSE_FILE" "$BACKUP_DIR/compose.before.yml" || die "Cannot back up Compose file."; docker inspect "$CID" > "$BACKUP_DIR/container.inspect.json" || die "Cannot record container configuration."; printf '%s\n' "$MOUNT_TYPE|$MOUNT_NAME|$MOUNT_SOURCE|$DATA_PATH" > "$BACKUP_DIR/mount.txt"
log "Archiving Server data mount..."; archive_mount || die "Data backup failed; old service remains running."; sha256sum "$BACKUP_DIR/data.tar.gz" > "$BACKUP_DIR/data.tar.gz.sha256" || die "Cannot checksum data backup."
if ! docker pull "$TARGET_IMAGE"; then
    if docker image inspect "$TARGET_IMAGE" >/dev/null 2>&1; then
        log "Warning: docker pull failed, but target image exists locally. Proceeding with local image."
    else
        die "Could not pull target image; old service remains running."
    fi
fi
printf 'services:\n  %s:\n    image: %s\n' "$SERVICE" "$TARGET_IMAGE" > "$OVERRIDE_FILE" || die "Could not create image override."; OVERRIDE_CREATED=1
compose_target up -d --no-build "$SERVICE" || abort "Compose could not recreate the target service"; sleep 3
CID=$(compose_target ps -q "$SERVICE"); [ -n "$CID" ] || abort "target container was not created"; [ "$(docker inspect --format '{{.State.Running}}' "$CID")" = true ] || abort "target container is not running"; docker exec "$CID" curl -fsS --max-time 10 "http://127.0.0.1:$PORT/" >/dev/null || abort "target Server health endpoint did not respond"
log "Server migration verified. Keep $OVERRIDE_FILE for future Compose commands. Backup retained at $BACKUP_DIR"
