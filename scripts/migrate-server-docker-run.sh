#!/usr/bin/env bash
# Migrate a single docker run Server container to Sonar without Compose or Coolify.
set -u -o pipefail
CONTAINER=""; TARGET_IMAGE=""; DATA_PATH="/app/data"; PORT=25774; BACKUP_ROOT="/var/backups/sonar-server-docker-run"; BACKUP_DIR=""; OLD_NAME=""; MOUNT_TYPE=""; MOUNT_NAME=""; MOUNT_SOURCE=""; DRY_RUN=0
die(){ echo "[sonar-server-docker-run] ERROR: $*" >&2; exit 1; }; log(){ echo "[sonar-server-docker-run] $*"; }
usage(){ cat <<'EOF'
Usage: sudo bash migrate-server-docker-run.sh --container NAME --target-image IMAGE [--data-path /app/data] [--port 25774] [--backup-root PATH] [--dry-run]
Backs up the named container's persistent data, stops but preserves the old
container, recreates the same runtime configuration with the new image, and
rolls back container name/image/data when validation fails.

  --dry-run                 Validate and print the plan without pulling or changing anything
EOF
}
while [ $# -gt 0 ]; do case "$1" in --container) CONTAINER=$2;shift;;--target-image) TARGET_IMAGE=$2;shift;;--data-path) DATA_PATH=$2;shift;;--port) PORT=$2;shift;;--backup-root) BACKUP_ROOT=$2;shift;;--dry-run) DRY_RUN=1;;-h|--help) usage;exit;;*) die "Unknown option: $1";;esac;shift;done
[ "${EUID:-$(id -u)}" -eq 0 ] || die "Run as root."; command -v docker >/dev/null || die "docker is required"; command -v python3 >/dev/null || die "python3 is required"; command -v curl >/dev/null || die "curl is required"; command -v tar >/dev/null || die "tar is required"; command -v find >/dev/null || die "find is required"
[ -n "$CONTAINER" ] && [ -n "$TARGET_IMAGE" ] || { usage; exit 1; }; docker inspect "$CONTAINER" >/dev/null 2>&1 || die "Container not found: $CONTAINER"
CID=$(docker inspect -f '{{.Id}}' "$CONTAINER"); docker inspect "$CONTAINER" > /tmp/sonar-inspect.$$; trap 'rm -f /tmp/sonar-inspect.$$' EXIT
IFS='|' read -r MOUNT_TYPE MOUNT_NAME MOUNT_SOURCE < <(python3 - "$DATA_PATH" /tmp/sonar-inspect.$$ <<'PY'
import json, sys
for mount in json.load(open(sys.argv[2]))[0].get("Mounts", []):
    if mount.get("Destination") == sys.argv[1]:
        print("|".join(str(mount.get(k, "")) for k in ("Type", "Name", "Source")))
        break
PY
); [ -n "$MOUNT_TYPE" ] || die "No persistent mount at $DATA_PATH"
if [ "$DRY_RUN" -eq 1 ]; then
    log "Plan: container=$CONTAINER; old-image=$(docker inspect --format '{{.Config.Image}}' "$CONTAINER"); target=$TARGET_IMAGE; data=$MOUNT_TYPE:$MOUNT_SOURCE; backup-root=$BACKUP_ROOT"
    log "Dry run finished; no changes were made and the target image was not pulled."
    exit 0
fi
ID=$(date -u +%Y%m%dT%H%M%SZ); BACKUP_DIR="$BACKUP_ROOT/$ID"; mkdir -p "$BACKUP_DIR"; cp /tmp/sonar-inspect.$$ "$BACKUP_DIR/container.inspect.json"
case "$MOUNT_TYPE" in volume) docker run --rm -v "$MOUNT_NAME":/data:ro -v "$BACKUP_DIR":/backup alpine:3.21 tar -C /data -czf /backup/data.tar.gz .;;bind) tar -C "$MOUNT_SOURCE" -czf "$BACKUP_DIR/data.tar.gz" .;;*) die "Unsupported mount type: $MOUNT_TYPE";;esac
docker pull "$TARGET_IMAGE" || die "Target image pull failed; old container is unchanged."
OLD_NAME="${CONTAINER}.pre-migration-${ID}"; docker stop "$CONTAINER"; docker rename "$CONTAINER" "$OLD_NAME"
payload(){ python3 - "$TARGET_IMAGE" /tmp/sonar-inspect.$$ <<'PY'
import json, sys
x = json.load(open(sys.argv[2]))[0]
c = x["Config"]
allowed = {"Aliases", "Links", "IPAMConfig", "MacAddress", "DriverOpts"}
endpoints = {name: {k:v for k,v in value.items() if k in allowed} for name,value in x.get("NetworkSettings", {}).get("Networks", {}).items()}
print(json.dumps({"Image":sys.argv[1], "Hostname":c.get("Hostname"), "User":c.get("User"), "Env":c.get("Env"), "Cmd":c.get("Cmd"), "Entrypoint":c.get("Entrypoint"), "WorkingDir":c.get("WorkingDir"), "Labels":c.get("Labels"), "ExposedPorts":c.get("ExposedPorts"), "HostConfig":x.get("HostConfig", {}), "NetworkingConfig":{"EndpointsConfig":endpoints}}))
PY
}
rollback(){ log "Rolling back..."; docker rm -f "$CONTAINER" >/dev/null 2>&1 || true; docker rename "$OLD_NAME" "$CONTAINER" >/dev/null 2>&1 || true; case "$MOUNT_TYPE" in volume) docker run --rm -v "$MOUNT_NAME":/data -v "$BACKUP_DIR":/backup alpine:3.21 sh -c 'rm -rf /data/* /data/.[!.]* /data/..?* 2>/dev/null || true; tar -C /data -xzf /backup/data.tar.gz';;bind) find "$MOUNT_SOURCE" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +; tar -C "$MOUNT_SOURCE" -xzf "$BACKUP_DIR/data.tar.gz";;esac; docker start "$CONTAINER" >/dev/null || true; }
curl --unix-socket /var/run/docker.sock -fsS -H 'Content-Type: application/json' -X POST "http://localhost/containers/create?name=$CONTAINER" --data-binary @<(payload) >/dev/null || { rollback; die "Could not create target container"; }
docker start "$CONTAINER" >/dev/null || { rollback; die "Could not start target container"; }; sleep 3; docker inspect -f '{{.State.Running}}' "$CONTAINER" | grep -qx true || { rollback; die "Target is not running"; }; docker exec "$CONTAINER" curl -fsS --max-time 10 "http://127.0.0.1:$PORT/" >/dev/null || { rollback; die "Target health check failed"; }
log "Migration verified. Old container retained as $OLD_NAME; backup retained at $BACKUP_DIR"
