#!/bin/sh
set -eu

repo="Costroid/costroid-sync"
module_path="github.com/costroid/costroid-sync"
binary="costroid-sync"
install_dir="${INSTALL_DIR:-/usr/local/bin}"
version="${VERSION:-latest}"

err() {
  printf '%s\n' "$*" >&2
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    err "Required command not found: $1"
    exit 1
  fi
}

detect_os() {
  case "$(uname -s 2>/dev/null || printf unknown)" in
    Linux) printf linux ;;
    Darwin) printf darwin ;;
    MINGW*|MSYS*|CYGWIN*) printf windows ;;
    *) printf unknown ;;
  esac
}

detect_arch() {
  case "$(uname -m 2>/dev/null || printf unknown)" in
    x86_64|amd64) printf amd64 ;;
    arm64|aarch64) printf arm64 ;;
    *) printf unknown ;;
  esac
}

download() {
  url="$1"
  dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$dest"
    return
  fi
  err "Required command not found: curl or wget"
  exit 1
}

install_binary() {
  src="$1"
  dest="$install_dir/$binary"
  if [ -d "$install_dir" ] && [ -w "$install_dir" ]; then
    cp "$src" "$dest"
    chmod 755 "$dest"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo mkdir -p "$install_dir"
    sudo cp "$src" "$dest"
    sudo chmod 755 "$dest"
    return
  fi
  err "Cannot write to $install_dir and sudo is not available."
  err "Install manually:"
  err "  mkdir -p \"\$HOME/.local/bin\""
  err "  cp \"$src\" \"\$HOME/.local/bin/$binary\""
  err "  chmod 755 \"\$HOME/.local/bin/$binary\""
  exit 1
}

need_cmd uname
os="$(detect_os)"
arch="$(detect_arch)"

if [ "$os" != linux ]; then
  err "prebuilt binaries are not available yet for $os."
  err "Install from source instead:"
  err "  go install $module_path@latest"
  exit 1
fi

if [ "$arch" != amd64 ] && [ "$arch" != arm64 ]; then
  err "Unsupported Linux architecture: $arch"
  err "Supported prebuilt architectures: amd64, arm64"
  err "Install from source instead:"
  err "  go install $module_path@latest"
  exit 1
fi

need_cmd tar
need_cmd mktemp

asset="${binary}_linux_${arch}.tar.gz"
if [ "$version" = latest ]; then
  url="https://github.com/$repo/releases/latest/download/$asset"
else
  url="https://github.com/$repo/releases/download/$version/$asset"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT HUP TERM

archive="$tmp/$asset"
download "$url" "$archive"
tar -xzf "$archive" -C "$tmp"

if [ ! -f "$tmp/$binary" ]; then
  err "Archive did not contain $binary."
  exit 1
fi

install_binary "$tmp/$binary"
printf 'Installed %s to %s/%s\n' "$binary" "$install_dir" "$binary"
