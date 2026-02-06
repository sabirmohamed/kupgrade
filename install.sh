#!/bin/bash
# kupgrade installer
# Usage: curl -fsSL https://raw.githubusercontent.com/sabirmohamed/kupgrade/main/install.sh | bash

set -e

REPO="sabirmohamed/kupgrade"
BINARY_NAME="kupgrade"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  mingw*|msys*|cygwin*) OS="windows" ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest release version (includes prereleases)
echo "Fetching latest release..."
VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases" | grep '"tag_name"' | head -1 | sed -E 's/.*"v([^"]+)".*/\1/')

if [ -z "$VERSION" ]; then
  echo "Failed to fetch latest version"
  exit 1
fi

echo "Installing kupgrade v${VERSION}..."

# Download
FILENAME="${BINARY_NAME}_v${VERSION}_${OS}_${ARCH}"
if [ "$OS" = "windows" ]; then
  FILENAME="${FILENAME}.zip"
else
  FILENAME="${FILENAME}.tar.gz"
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/v${VERSION}/${FILENAME}"
TMP_DIR=$(mktemp -d)

echo "Downloading ${DOWNLOAD_URL}..."
curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/${FILENAME}"

# Extract
cd "$TMP_DIR"
if [ "$OS" = "windows" ]; then
  unzip -q "$FILENAME"
else
  tar -xzf "$FILENAME"
fi

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "$BINARY_NAME" "$INSTALL_DIR/"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "$BINARY_NAME" "$INSTALL_DIR/"
fi

# Cleanup
rm -rf "$TMP_DIR"

echo ""
echo "kupgrade v${VERSION} installed successfully!"
echo ""
echo "Usage:"
echo "  kupgrade watch       # start upgrade observer"
echo "  kupgrade version     # check version"
echo ""
