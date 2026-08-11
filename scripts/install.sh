#!/usr/bin/env sh
set -eu

REPO="jazho76/uplink"
ASSET="uplink-linux-amd64"
INSTALL_DIR="${HOME}/.local/bin"
TARGET="${INSTALL_DIR}/uplink"

os="$(uname -s)"
arch="$(uname -m)"

if [ "$os" != "Linux" ]; then
  echo "uplink ships a Linux binary only (got: $os)." >&2
  exit 1
fi

if [ "$arch" != "x86_64" ]; then
  echo "uplink ships an amd64 binary only (got: $arch)." >&2
  echo "Build from source instead: https://github.com/${REPO}" >&2
  exit 1
fi

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

echo "Installed uplink. Add a VM template with: uplink vm template add <git-url>"
echo "On GNOME, register the launcher shortcuts with: uplink install-shortcuts"
