#!/bin/bash
# ccc installation script
# Downloads the latest binary for your platform

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Detect platform
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Darwin)
        OS="darwin"
        ;;
    Linux)
        OS="linux"
        ;;
    *)
        echo -e "${RED}Error: Unsupported OS: $OS${NC}"
        echo "ccc supports macOS and Linux. Windows users should use WSL."
        exit 1
        ;;
esac

case "$ARCH" in
    x86_64|amd64)
        ARCH="amd64"
        ;;
    arm64|aarch64)
        ARCH="arm64"
        ;;
    armv7l)
        ARCH="arm"
        ;;
    *)
        echo -e "${RED}Error: Unsupported architecture: $ARCH${NC}"
        echo "ccc supports amd64, arm64, and arm/v7."
        exit 1
        ;;
esac

# Get latest version from GitHub API
echo "Fetching latest version..."
LATEST_VERSION=$(curl -s https://api.github.com/repos/tuannvm/ccc/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' | sed 's/^v//')

if [ -z "$LATEST_VERSION" ]; then
    echo -e "${RED}Error: Could not fetch latest version from GitHub.${NC}"
    echo "Please check your internet connection or visit:"
    echo "  https://github.com/tuannvm/ccc/releases/latest"
    exit 1
fi

ARCHIVE_NAME="ccc_${LATEST_VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/tuannvm/ccc/releases/latest/download/${ARCHIVE_NAME}"

INSTALL_DIR="/usr/local/bin"

# Check if installation directory is writable
if [ ! -w "$INSTALL_DIR" ]; then
    echo -e "${YELLOW}Warning: $INSTALL_DIR is not writable.${NC}"
    echo "Installing to ~/.local/bin instead..."
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
fi

# Create temp directory
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

echo -e "${GREEN}Installing ccc ${LATEST_VERSION}...${NC}"
echo "Platform: $OS $ARCH"
echo "Download: $ARCHIVE_NAME"
echo "Install to: $INSTALL_DIR"
echo ""

# Download and extract
echo -n "Downloading..."
if curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/$ARCHIVE_NAME"; then
    echo -e " ${GREEN}done${NC}"
else
    echo -e " ${RED}failed${NC}"
    echo ""
    echo "Manual download instructions:"
    echo "  1. Visit: https://github.com/tuannvm/ccc/releases/latest"
    echo "  2. Download: $ARCHIVE_NAME"
    echo "  3. Extract: tar -xzf $ARCHIVE_NAME"
    echo "  4. Move to: $INSTALL_DIR/ccc"
    echo "  5. Make executable: chmod +x $INSTALL_DIR/ccc"
    exit 1
fi

echo -n "Extracting..."
if tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR"; then
    echo -e " ${GREEN}done${NC}"
else
    echo -e " ${RED}failed${NC}"
    exit 1
fi

# Install binary
echo -n "Installing..."
if mv "$TMP_DIR/ccc" "$INSTALL_DIR/ccc"; then
    echo -e " ${GREEN}done${NC}"
else
    echo -e " ${RED}failed${NC}"
    exit 1
fi

# Make executable
chmod +x "$INSTALL_DIR/ccc"

# Verify installation
if command -v ccc &> /dev/null; then
    echo ""
    echo -e "${GREEN}✓ ccc installed successfully!${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Create a Telegram bot: @BotFather → /newbot"
    echo "  2. Setup ccc: ccc setup YOUR_BOT_TOKEN"
    echo "  3. Start coding: ccc"
    echo ""
    echo "For more info, visit: https://github.com/tuannvm/ccc"
else
    echo ""
    echo -e "${YELLOW}Installation complete, but ccc is not in PATH.${NC}"
    echo "Add this to your ~/.bashrc or ~/.zshrc:"
    echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
fi
