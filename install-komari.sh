#!/bin/bash
# Backward compatibility wrapper for Sonar installer
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd)"
if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/install-sonar.sh" ]; then
    echo "Note: Komari is now Sonar. Launching install-sonar.sh..."
    exec bash "$SCRIPT_DIR/install-sonar.sh" "$@"
else
    echo "Note: Komari is now Sonar. Fetching install-sonar.sh..."
    exec bash -c "$(curl -fsSL https://raw.githubusercontent.com/sonar-probe/sonar/main/install-sonar.sh)" -- "$@"
fi
