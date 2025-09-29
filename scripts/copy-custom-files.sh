#!/bin/bash

# Generic Custom Files Copy Script
# This script copies custom files from the custom directory to any generated SDK
# Usage: ./copy-custom-files.sh <sdk-type>
# Example: ./copy-custom-files.sh javascript

set -e -o pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to show usage
show_usage() {
    echo -e "${BLUE}Usage: $0 <sdk-type>${NC}"
    echo -e "${YELLOW}Supported SDK types:${NC}"
    echo -e "  javascript  - JavaScript/TypeScript SDK"
    echo -e "  python      - Python SDK"
    echo -e "  go          - Go SDK"
    echo ""
    echo -e "${YELLOW}Examples:${NC}"
    echo -e "  $0 javascript"
    echo -e "  $0 python"
    echo -e "  $0 go"
    exit 1
}

# Check if SDK type is provided
if [ $# -eq 0 ]; then
    echo -e "${RED}❌ Error: SDK type is required${NC}"
    show_usage
fi

SDK_TYPE="$1"

# Configuration based on SDK type
case "$SDK_TYPE" in
    "javascript")
        CUSTOM_DIR="api/custom/javascript"
        TARGET_DIR="api/javascript"
        SDK_NAME="JavaScript/TypeScript SDK"
        ;;
    "python")
        CUSTOM_DIR="api/custom/python"
        TARGET_DIR="api/python"
        SDK_NAME="Python SDK"
        ;;
    "go")
        CUSTOM_DIR="api/custom/go"
        TARGET_DIR="api/go"
        SDK_NAME="Go SDK"
        ;;
    *)
        echo -e "${RED}❌ Error: Unsupported SDK type: $SDK_TYPE${NC}"
        show_usage
        ;;
esac

echo -e "${BLUE}🔄 Copying custom files to $SDK_NAME...${NC}"

# Check if custom directory exists
if [ ! -d "$CUSTOM_DIR" ]; then
    echo -e "${YELLOW}⚠️  No custom directory found at $CUSTOM_DIR${NC}"
    echo -e "${YELLOW}💡 Custom files will not be copied${NC}"
    exit 0
fi

# Check if target directory exists
if [ ! -d "$TARGET_DIR" ]; then
    echo -e "${RED}❌ Error: Target directory not found at $TARGET_DIR${NC}"
    echo -e "${YELLOW}💡 Please run 'make generate-${SDK_TYPE}-sdk' first${NC}"
    exit 1
fi

# Check if there are any custom files to copy
if [ -z "$(find "$CUSTOM_DIR" -type f -not -name "README.md" 2>/dev/null)" ]; then
    echo -e "${YELLOW}⚠️  No custom files found to copy${NC}"
    echo -e "${YELLOW}💡 Add custom files to $CUSTOM_DIR to include them in the SDK${NC}"
    exit 0
fi

# Copy custom files
echo -e "${BLUE}📂 Found custom files, copying to generated SDK...${NC}"
files_copied=0

# Find all files in the custom directory (excluding README files)
while IFS= read -r -d '' file; do
    # Skip README files in the root of custom directory
    if [[ "$file" == *"/README.md" ]] && [[ "$(dirname "$file")" == "$CUSTOM_DIR" ]]; then
        continue
    fi
    
    # Calculate relative path from custom directory
    rel_path="${file#$CUSTOM_DIR/}"
    
    # Create target file path
    target_file="$TARGET_DIR/$rel_path"
    
    # Create target directory if it doesn't exist
    target_file_dir="$(dirname "$target_file")"
    mkdir -p "$target_file_dir"
    
    # Copy the file
    cp "$file" "$target_file"
    echo -e "${GREEN}✅ Copied: $rel_path${NC}"
    ((files_copied++))
    
done < <(find "$CUSTOM_DIR" -type f -print0)

echo -e "${GREEN}📁 Total files copied: $files_copied${NC}"
echo -e "${GREEN}✅ Custom files copy complete!${NC}"
echo -e "${BLUE}💡 Custom files have been copied to $TARGET_DIR${NC}"
