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

# Nothing else is stopped here. A sweep by NAME matches every process whose
# command line contains "modelsdb", which includes a developer instance, an
# agent's sandbox, and the maintainer's own running server on another port -
# all of which this build has no business stopping. The PID file above names
# the one process a build may replace.
#
# On Windows a running .exe cannot be overwritten, so the PID-file stop above
# is what makes the build possible there. On Unix a new file simply replaces
# the old one and nothing needs stopping at all.

# Inject the app version (single source: the repo-root VERSION file). Data/config/
# cache paths are resolved at RUNTIME (OS-standard dirs or a config.jsonc), never
# baked in, so the binary is portable across machines.
LDFLAGS="-X 'modelsdb/internal/shared.Version=$APP_VERSION'"

# The release binaries are built with -trimpath and CGO off so the same commit and
# Go toolchain give the same bytes on any machine. `go run ./cmd/sign publish`
# rebuilds both binaries from the tag's commit with these exact flags and refuses
# to sign unless the hashes equal CI's. Change a flag here and in
# cmd/sign/rebuild.go together.

echo ""
echo "Minifying static assets..."
# Regenerates internal/app/static-min/ (git-ignored) from internal/app/static/:
# app.js and style.css minified, every other file (the vendored jQuery/
# DataTables bundles, the favicon) copied through unchanged. The -tags
# minified build below embeds THIS directory instead of static/, so only the
# shipped binary serves minified assets - the repo's own app.js/style.css
# stay human-authored and readable. `go run ./cmd/modelsdb` (dev mode) never
# runs this step and never sees static-min/.
go run ./cmd/minifyassets
if [ $? -ne 0 ]; then
    echo "ERROR: asset minification failed!"
    exit 1
fi

echo ""
echo "Building for Windows (modelsdb.exe)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -tags minified -ldflags "$LDFLAGS" -o dist/modelsdb.exe ./cmd/modelsdb
if [ $? -ne 0 ]; then
    echo "ERROR: Windows build failed!"
    exit 1
fi
echo "  [ok] Windows binary created"

echo "Building for Linux (modelsdb)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -tags minified -ldflags "$LDFLAGS" -o dist/modelsdb ./cmd/modelsdb
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
