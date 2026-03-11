#!/bin/bash
# run-automated.sh - Fully automated demo using the REST API

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BINARY="$PROJECT_ROOT/dist/tui-streamer"

PORT="${1:-8080}"
API_URL="http://localhost:$PORT"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}🤖 tui-streamer Automated Demo${NC}"
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

echo -e "${YELLOW}Starting tui-streamer server on port $PORT...${NC}"

# Start server in background
"$BINARY" -port "$PORT" -open > /dev/null 2>&1 &
SERVER_PID=$!

# Function to cleanup on exit
cleanup() {
    echo ""
    echo -e "${YELLOW}Stopping server...${NC}"
    kill $SERVER_PID 2>/dev/null || true
    wait $SERVER_PID 2>/dev/null || true
    echo -e "${GREEN}✓ Server stopped${NC}"
}
trap cleanup EXIT

# Wait for server to start
echo -e "${YELLOW}Waiting for server to start...${NC}"
sleep 2

# Check if server is running
if ! curl -s "$API_URL/api/sessions" > /dev/null 2>&1; then
    echo -e "${RED}❌ Error: Server failed to start${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Server is running${NC}"
echo ""

# Create a session
echo -e "${BLUE}📝 Creating session...${NC}"
SESSION_RESPONSE=$(curl -s -X POST "$API_URL/api/sessions" \
    -H "Content-Type: application/json" \
    -d '{"name":"lorem-demo"}')

SESSION_ID=$(echo "$SESSION_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

if [ -z "$SESSION_ID" ]; then
    echo -e "${RED}❌ Error: Failed to create session${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Session created: $SESSION_ID${NC}"
echo ""

# Execute the command
echo -e "${BLUE}🚀 Executing lorem ipsum fetcher...${NC}"
EXEC_RESPONSE=$(curl -s -X POST "$API_URL/api/sessions/$SESSION_ID/exec" \
    -H "Content-Type: application/json" \
    -d "{\"command\":\"$SCRIPT_DIR/fetch-article.sh\"}")

echo -e "${GREEN}✓ Command dispatched${NC}"
echo ""

echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✅ Demo is running!${NC}"
echo ""
echo -e "${YELLOW}View the output at: ${BLUE}$API_URL${NC}"
echo -e "${YELLOW}WebSocket URL: ${BLUE}ws://localhost:$PORT/ws/$SESSION_ID${NC}"
echo ""
echo -e "${CYAN}The browser should have opened automatically.${NC}"
echo -e "${CYAN}Press Ctrl+C to stop the server when done.${NC}"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Keep script running until interrupted
wait $SERVER_PID
