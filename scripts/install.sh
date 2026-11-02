#!/usr/bin/env sh
set -eu

REPO="jazho76/uplink"
INSTALL_DIR="${HOME}/.local/bin"
TARGET="${INSTALL_DIR}/uplink"

kernel="$(uname -s)"
machine="$(uname -m)"

case "$kernel" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *)
    echo "uplink ships Linux and macOS binaries only (got: $kernel)." >&2
    echo "Build from source instead: https://github.com/${REPO}" >&2
    exit 1
    ;;
esac

case "$machine" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *)
    echo "uplink ships amd64 and arm64 binaries only (got: $machine)." >&2
    echo "Build from source instead: https://github.com/${REPO}" >&2
    exit 1
    ;;
esac

ASSET="uplink-${os}-${arch}"
url="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "Downloading uplink -> ${TARGET}"
mkdir -p "$INSTALL_DIR"
if command -v curl >/dev/null 2>&1; then
  curl -fSL "$url" -o "$TARGET"
elif command -v wget >/dev/null 2>&1; then
  wget -O "$TARGET" "$url"
else
  echo "Need curl or wget to download uplink." >&2
  exit 1
fi
chmod +x "$TARGET"

case ":${PATH}:" in
  *:"${INSTALL_DIR}":*) ;;
  *) echo "Note: ${INSTALL_DIR} is not on your PATH. Add it: export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac

echo "Installed uplink. Declare your targets with: uplink config edit"
if [ "$os" = "linux" ]; then
  echo "On GNOME, bind Ctrl+Alt+T to the dashboard with: uplink install-shortcut"
fi
