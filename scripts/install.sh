#!/usr/bin/env sh
set -eu

REPO="jazho76/vmm"
ASSET="vmm-linux-amd64"
INSTALL_DIR="${HOME}/.local/bin"
TARGET="${INSTALL_DIR}/vmm"

os="$(uname -s)"
arch="$(uname -m)"

if [ "$os" != "Linux" ]; then
  echo "vmm ships a Linux binary only (got: $os)." >&2
  exit 1
fi

if [ "$arch" != "x86_64" ]; then
  echo "vmm ships an amd64 binary only (got: $arch)." >&2
  echo "Build from source instead: https://github.com/${REPO}" >&2
  exit 1
fi

url="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "Downloading vmm -> ${TARGET}"
mkdir -p "$INSTALL_DIR"
if command -v curl >/dev/null 2>&1; then
  curl -fSL "$url" -o "$TARGET"
elif command -v wget >/dev/null 2>&1; then
  wget -O "$TARGET" "$url"
else
  echo "Need curl or wget to download vmm." >&2
  exit 1
fi
chmod +x "$TARGET"

case ":${PATH}:" in
  *:"${INSTALL_DIR}":*) ;;
  *) echo "Note: ${INSTALL_DIR} is not on your PATH. Add it: export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac

echo "Installed vmm. Add a template with: vmm template add <git-url> <name>"
echo "On GNOME, register the launcher shortcuts with: vmm install-shortcuts"
