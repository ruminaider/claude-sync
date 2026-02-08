#!/bin/sh
set -e

# Install claude-sync — https://github.com/ruminaider/claude-sync
# Usage: curl -fsSL https://raw.githubusercontent.com/ruminaider/claude-sync/main/install.sh | sh

REPO="ruminaider/claude-sync"
BINARY="claude-sync"
INSTALL_DIR="${CLAUDE_SYNC_INSTALL_DIR:-$HOME/.local/bin}"

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)       ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  darwin|linux) ;;
  *)
    echo "Error: unsupported OS: $OS"
    exit 1
    ;;
esac

# Determine version
if [ -n "$1" ]; then
  VERSION="$1"
else
  VERSION=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | cut -d'"' -f4)
  if [ -z "$VERSION" ]; then
    echo "Error: could not determine latest version"
    exit 1
  fi
fi

ASSET="${BINARY}-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

echo "Installing ${BINARY} ${VERSION} (${OS}/${ARCH})..."

# Download
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

HTTP_CODE=$(curl -sSL -w '%{http_code}' -o "${TMPDIR}/${BINARY}" "$URL")
if [ "$HTTP_CODE" != "200" ]; then
  echo "Error: download failed (HTTP ${HTTP_CODE})"
  echo "URL: ${URL}"
  echo ""
  echo "Available at: https://github.com/${REPO}/releases"
  exit 1
fi

chmod +x "${TMPDIR}/${BINARY}"

# Install
mkdir -p "$INSTALL_DIR"
mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

echo "Installed to ${INSTALL_DIR}/${BINARY}"

# Verify PATH
if ! echo "$PATH" | tr ':' '\n' | grep -q "^${INSTALL_DIR}$"; then
  echo ""
  echo "Add ${INSTALL_DIR} to your PATH:"
  SHELL_NAME=$(basename "${SHELL:-/bin/sh}")
  case "$SHELL_NAME" in
    zsh)  echo "  echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.zshrc && source ~/.zshrc" ;;
    bash) echo "  echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.bashrc && source ~/.bashrc" ;;
    fish) echo "  fish_add_path ${INSTALL_DIR}" ;;
    *)    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\"" ;;
  esac
else
  echo ""
  "${INSTALL_DIR}/${BINARY}" version
  echo "Ready to use! Run: ${BINARY} --help"
fi
