#!/bin/bash
# fetch-article.sh - Fetch a lorem ipsum article and output it line by line
# This demonstrates streaming output with tui-streamer

set -e

# Colors for output
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default article ID
ARTICLE_ID="${1:-foo}"
API_URL="https://lorem-api.com/api/article/${ARTICLE_ID}"

echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}📰 Lorem Ipsum Article Streamer${NC}"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

echo -e "${YELLOW}Fetching article: ${ARTICLE_ID}${NC}"
echo -e "${YELLOW}URL: ${API_URL}${NC}"
echo ""

# Fetch the article
echo "⏳ Downloading article..."
RESPONSE=$(curl -s "$API_URL")

# Check if curl succeeded
if [ $? -ne 0 ]; then
    echo "❌ Error: Failed to fetch article from API"
    exit 1
fi

# Parse the JSON response and extract fields
if command -v jq &> /dev/null; then
    # Use jq if available (better formatting)
    TITLE=$(echo "$RESPONSE" | jq -r '.title // "Untitled"')
    CONTENT=$(echo "$RESPONSE" | jq -r '.content // "No content available"')
    AUTHOR=$(echo "$RESPONSE" | jq -r '.author // "Unknown"')
else
    # Fallback to basic parsing without jq
    TITLE=$(echo "$RESPONSE" | grep -o '"title":"[^"]*"' | cut -d'"' -f4 | head -1)
    CONTENT=$(echo "$RESPONSE" | grep -o '"content":"[^"]*"' | cut -d'"' -f4 | head -1)
    AUTHOR=$(echo "$RESPONSE" | grep -o '"author":"[^"]*"' | cut -d'"' -f4 | head -1)

    # Decode escaped characters
    TITLE=$(echo -e "$TITLE")
    CONTENT=$(echo -e "$CONTENT")
    AUTHOR=$(echo -e "$AUTHOR")
fi

echo ""
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}Title:${NC} $TITLE"
echo -e "${GREEN}Author:${NC} $AUTHOR"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Stream the content word by word with a small delay for visual effect
echo -e "${GREEN}📖 Article Content:${NC}"
echo ""

# Split content into words and stream them
echo "$CONTENT" | fold -w 70 -s | while IFS= read -r line; do
    echo "$line"
    # Small delay to simulate streaming (remove for actual fast streaming)
    sleep 0.1
done

echo ""
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✅ Article streaming complete!${NC}"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
