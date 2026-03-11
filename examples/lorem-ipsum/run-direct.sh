#!/bin/bash
# run-direct.sh - Run the lorem ipsum example with tui-streamer binary directly

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BINARY="$PROJECT_ROOT/dist/tui-streamer"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}🚀 tui-streamer Lorem Ipsum Demo (Direct Binary)${NC}"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Check if binary exists
if [ ! -f "$BINARY" ]; then
    echo -e "${RED}❌ Error: tui-streamer binary not found at $BINARY${NC}"
    echo -e "${YELLOW}💡 Build it first with: make build${NC}"
    exit 1
fi

# Make sure the script is executable
chmod +x "$SCRIPT_DIR/fetch-article.sh"

echo -e "${GREEN}✓ Found binary: $BINARY${NC}"
echo -e "${GREEN}✓ Script ready: $SCRIPT_DIR/fetch-article.sh${NC}"
echo ""
echo -e "${YELLOW}Starting tui-streamer on http://localhost:8080${NC}"
echo -e "${YELLOW}Press Ctrl+C to stop the server${NC}"
echo ""
echo -e "${CYAN}Next steps:${NC}"
echo "  1. Wait for the browser to open"
echo "  2. Click 'Create New Session'"
echo "  3. Enter a session name (e.g. 'lorem-demo')"
echo "  4. In the command input, enter:"
echo -e "     ${GREEN}$SCRIPT_DIR/fetch-article.sh${NC}"
echo "  5. Click 'Execute' and watch the article stream!"
echo ""
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Start the server with auto-open
cd "$PROJECT_ROOT"
"$BINARY" -open -port 8080
