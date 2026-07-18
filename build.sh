#!/bin/bash

echo "=== ModelsDB Build Script ==="
echo ""

echo "Reading version..."
# Get absolute path to the project root regardless of where the script is called from
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)

# Single source of truth for the app version: the repo-root VERSION file.
APP_VERSION=$(tr -d '[:space:]' < "$PROJECT_ROOT/VERSION")
echo "  -> Version: $APP_VERSION"

echo "Creating directories..."
mkdir -p dist

# Detect OS type for process killing
OS_TYPE="unix"
if command -v uname >/dev/null 2>&1; then
    UNAME=$(uname)
    if [[ "$UNAME" == *"MINGW"* ]] || [[ "$UNAME" == *"CYGWIN"* ]] || [[ "$UNAME" == *"MSYS"* ]]; then
        OS_TYPE="windows"
    fi
fi

# Track if process was running before build
WAS_RUNNING=0

# Stop any running instance before building
echo "Checking for running instance..."

# Function to kill a process by PID (works on both Windows and Unix)
kill_process() {
    local pid=$1
    local os_type=$2
    
    if [ "$os_type" = "windows" ]; then
        # Use taskkill on Windows with //F and //PID syntax
        echo "  -> Stopping Windows process (PID: $pid)..."
        taskkill //F //PID "$pid" 2>/dev/null
    else
        # Use kill on Unix/Linux
        echo "  -> Stopping Unix process (PID: $pid)..."
        kill -9 "$pid" 2>/dev/null
    fi
}

# Check for PID file first
if [ -f "dist/.pid" ]; then
    OLD_PID=$(cat dist/.pid)
    echo "  -> Found PID file with PID: $OLD_PID"
    
    # Try to kill by PID
    echo "  -> Attempting to stop process..."
    kill_process "$OLD_PID" "$OS_TYPE"
    sleep 1
    
    # Verify it's dead
    if [ "$OS_TYPE" = "windows" ]; then
        if ! tasklist //FI "PID eq $OLD_PID" 2>/dev/null | grep -q "$OLD_PID"; then
            echo "  -> Process successfully stopped"
            WAS_RUNNING=1
        else
            echo "  -> Process still running, force killing..."
            kill_process "$OLD_PID" "$OS_TYPE"
            sleep 1
        fi
    else
        if ! kill -0 "$OLD_PID" 2>/dev/null; then
            echo "  -> Process successfully stopped"
            WAS_RUNNING=1
        else
            echo "  -> Process still running, force killing..."
            kill -9 "$OLD_PID" 2>/dev/null
            sleep 1
        fi
    fi
    # Always remove PID file after attempting to kill
    rm -f "dist/.pid"
    echo "  -> PID file cleaned up"
fi

# Secondary measure: kill any process named modelsdb
echo "  -> Checking for running modelsdb processes by name..."
if [ "$OS_TYPE" = "windows" ]; then
    # Windows: use tasklist and taskkill with // syntax
    MODELSDB_PID=$(tasklist //FI "IMAGENAME eq modelsdb.exe" 2>/dev/null | grep "modelsdb.exe" | awk '{print $2}')
    if [ -n "$MODELSDB_PID" ]; then
        echo "  -> Found modelsdb.exe (PID: $MODELSDB_PID), killing..."
        taskkill //F //PID "$MODELSDB_PID" 2>/dev/null
        sleep 1
    fi
else
    # Unix/Linux: use pkill or kill based on process name
    MODELSDB_PIDS=$(pgrep -f "modelsdb$" 2>/dev/null)
    if [ -n "$MODELSDB_PIDS" ]; then
        echo "  -> Found modelsdb process(es): $MODELSDB_PIDS"
        echo "$MODELSDB_PIDS" | while read pid; do
            kill -9 "$pid" 2>/dev/null
        done
        sleep 1
    fi
fi

# Also check for any process containing "modelsdb" in its name
echo "  -> Checking for any modelsdb-related processes..."
if [ "$OS_TYPE" = "windows" ]; then
    MODELSDB_ANY=$(tasklist //FI "IMAGENAME eq modelsdb*" 2>/dev/null | grep -i "modelsdb" | head -1 | awk '{print $2}')
    if [ -n "$MODELSDB_ANY" ]; then
        echo "  -> Found modelsdb process (PID: $MODELSDB_ANY), killing..."
        taskkill //F //PID "$MODELSDB_ANY" 2>/dev/null
    fi
else
    # Kill any process with "modelsdb" in the name (except this script)
    pkill -f "modelsdb" 2>/dev/null
    sleep 1
fi

# Inject the app version (single source: the repo-root VERSION file). Data/config/
# cache paths are resolved at RUNTIME (OS-standard dirs or a config.jsonc), never
# baked in, so the binary is portable across machines.
LDFLAGS="-X 'modelsdb/internal/shared.Version=$APP_VERSION'"

# Refresh the embedded seed catalog from the maintainer's live database when it is
# reachable, so a local release build embeds the freshest catalog. This only fires
# on a Unix-like shell where an installed 'modelsdb' is on PATH AND its live DB
# exists; CI / tag builds (no installed binary, no DB) skip it and embed the
# committed internal/seedcatalog/catalog.json. A failed export is a warning, not a
# build failure - the committed file is used instead. The already-installed binary
# is used, never the not-yet-built dist binary.
echo ""
echo "Refreshing embedded seed catalog..."
SEED_JSON="$PROJECT_ROOT/internal/seedcatalog/catalog.json"
LIVE_DB="$HOME/.local/share/modelsdb/modelsdb.db"
if [ "$OS_TYPE" = "unix" ] && command -v modelsdb >/dev/null 2>&1 && [ -f "$LIVE_DB" ]; then
    if modelsdb export "$SEED_JSON"; then
        echo "  -> Refreshed $SEED_JSON from the live database"
    else
        echo "  -> WARNING: export failed; using the committed catalog"
    fi
else
    echo "  -> No installed modelsdb + live DB; using the committed catalog"
fi

echo ""
echo "Building for Windows (modelsdb.exe)..."
GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/modelsdb.exe ./cmd/modelsdb
if [ $? -ne 0 ]; then
    echo "ERROR: Windows build failed!"
    exit 1
fi
echo "  [ok] Windows binary created"

echo "Building for Linux (modelsdb)..."
GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/modelsdb ./cmd/modelsdb
if [ $? -ne 0 ]; then
    echo "ERROR: Linux build failed!"
    exit 1
fi
echo "  [ok] Linux binary created"

echo ""
echo "=== Build Complete ==="
echo "Binaries saved to dist/"
echo "Version $APP_VERSION injected via LDFLAGS; paths resolved at runtime."
echo ""

# Restart the process if it was running before the build
if [ "$WAS_RUNNING" -eq 1 ]; then
    echo "Restarting ModelsDB..."
    # Relaunch with the explicit 'serve' subcommand so the restart always starts
    # the server, never the interactive menu (which a no-arg terminal launch shows).
    if [ "$OS_TYPE" = "windows" ]; then
        cd dist
        start "" ./modelsdb.exe serve
        cd ..
        echo "  -> ModelsDB restarted"
    else
        nohup ./dist/modelsdb serve > /dev/null 2>&1 < /dev/null &
        echo "  -> ModelsDB restarted"
    fi
fi

echo ""
echo "To run:"
echo "  Windows: dist\modelsdb.exe"
echo "  Linux:   ./dist/modelsdb"
