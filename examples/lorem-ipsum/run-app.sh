#!/bin/bash
# run-app.sh - Run the lorem ipsum example with the packaged macOS app

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
APP_BUNDLE="$PROJECT_ROOT/dist/TuiStreamer.app"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}🚀 tui-streamer Lorem Ipsum Demo (macOS App)${NC}"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Check if running on macOS
if [[ "$OSTYPE" != "darwin"* ]]; then
    echo -e "${RED}❌ Error: This script is for macOS only${NC}"
    echo -e "${YELLOW}💡 Use run-direct.sh instead${NC}"
    exit 1
fi

# Check if app bundle exists
if [ ! -d "$APP_BUNDLE" ]; then
    echo -e "${RED}❌ Error: App bundle not found at $APP_BUNDLE${NC}"
    echo -e "${YELLOW}💡 Build it first with: make app${NC}"
    exit 1
fi

# Make sure the script is executable
chmod +x "$SCRIPT_DIR/fetch-article.sh"

echo -e "${GREEN}✓ Found app bundle: $APP_BUNDLE${NC}"
echo -e "${GREEN}✓ Script ready: $SCRIPT_DIR/fetch-article.sh${NC}"
echo ""
echo -e "${YELLOW}Launching TuiStreamer.app...${NC}"
echo ""
echo -e "${CYAN}Next steps:${NC}"
echo "  1. Wait for the app window to open"
echo "  2. Click 'Create New Session'"
echo "  3. Enter a session name (e.g. 'lorem-demo')"
echo "  4. In the command input, enter:"
echo -e "     ${GREEN}$SCRIPT_DIR/fetch-article.sh${NC}"
echo "  5. Click 'Execute' and watch the article stream!"
echo ""
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Open the app
open "$APP_BUNDLE"

echo -e "${GREEN}✓ App launched!${NC}"
echo -e "${YELLOW}💡 The app will run in the background. Close the window to quit.${NC}"
