#!/bin/sh
set -eu

repo="Costroid/costroid-sync"
module_path="github.com/costroid/costroid-sync"
binary="costroid-sync"
install_dir="${INSTALL_DIR:-/usr/local/bin}"
version="${VERSION:-latest}"
docs_url="https://github.com/$repo/blob/main/docs/install.md"

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

detect_wsl() {
  if [ -r /proc/version ] && grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null; then
    return 0
  fi
  return 1
}

go_install_cmd() {
  if [ "$version" = latest ]; then
    printf 'go install %s@latest' "$module_path"
  else
    printf 'go install %s@%s' "$module_path" "$version"
  fi
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

verify_checksum() {
  archive_path="$1"
  asset_name="$2"
  sums_path="$3"

  hasher=""
  if command -v sha256sum >/dev/null 2>&1; then
    hasher="sha256sum"
  elif command -v shasum >/dev/null 2>&1; then
    hasher="shasum -a 256"
  else
    err "Warning: sha256sum/shasum not found - skipping checksum verification."
    return 0
  fi

  expected="$(awk -v name="$asset_name" '$2 == name { print $1; exit }' "$sums_path")"
  if [ -z "$expected" ]; then
    err "Warning: $asset_name not in checksums.txt - skipping verification."
    return 0
  fi

  actual="$($hasher "$archive_path" | awk '{print $1}')"
  if [ "$expected" != "$actual" ]; then
    err "Checksum verification failed for $asset_name."
    err "  expected: $expected"
    err "  actual:   $actual"
    exit 1
  fi
  printf 'Verified sha256 checksum for %s\n' "$asset_name"
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
  err "  # then ensure \$HOME/.local/bin is on your PATH"
  exit 1
}

need_cmd uname
os="$(detect_os)"
arch="$(detect_arch)"

if [ "$os" = darwin ]; then
  err "No prebuilt macOS binary yet (costroid-sync uses go-sqlite3, which requires CGO)."
  err "Install from source:"
  err "  1. Install Xcode Command Line Tools: xcode-select --install"
  err "  2. Install Go 1.24+:                 https://go.dev/dl/"
  err "  3. Run:                              $(go_install_cmd)"
  err "Details: $docs_url"
  exit 1
fi

if [ "$os" = windows ]; then
  err "Run the PowerShell installer on Windows:"
  err "  irm https://raw.githubusercontent.com/$repo/main/install.ps1 | iex"
  err ""
  err "Or, with Go 1.24+ and a C compiler (MinGW-w64) installed, from any shell:"
  err "  $(go_install_cmd)"
  err ""
  err "Details: $docs_url"
  exit 1
fi

if [ "$os" = unknown ]; then
  err "Unsupported OS: $(uname -s 2>/dev/null || printf unknown)"
  err "Try installing from source with Go 1.24+ and a C compiler:"
  err "  $(go_install_cmd)"
  err "Details: $docs_url"
  exit 1
fi

if [ "$arch" != amd64 ] && [ "$arch" != arm64 ]; then
  err "Unsupported Linux architecture: $arch"
  err "Supported prebuilt architectures: amd64, arm64"
  err "Install from source instead:"
  err "  $(go_install_cmd)"
  err "Details: $docs_url"
  exit 1
fi

if detect_wsl; then
  err "Detected WSL - using Linux build."
fi

need_cmd tar
need_cmd mktemp

asset="${binary}_linux_${arch}.tar.gz"
if [ "$version" = latest ]; then
  base_url="https://github.com/$repo/releases/latest/download"
else
  base_url="https://github.com/$repo/releases/download/$version"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT HUP TERM

archive="$tmp/$asset"
sums="$tmp/checksums.txt"
download "$base_url/$asset" "$archive"
download "$base_url/checksums.txt" "$sums"
verify_checksum "$archive" "$asset" "$sums"
tar -xzf "$archive" -C "$tmp"

if [ ! -f "$tmp/$binary" ]; then
  err "Archive did not contain $binary."
  exit 1
fi

install_binary "$tmp/$binary"
printf 'Installed %s to %s/%s\n' "$binary" "$install_dir" "$binary"
