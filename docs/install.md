# Installing costroid-sync

`costroid-sync` is distributed as:

- prebuilt Linux binaries (`amd64`, `arm64`) attached to each GitHub release
- a `go install` source path that works on any OS with Go 1.24+ and a C compiler

Native macOS and Windows binaries are **not** shipped yet. `costroid-sync` uses [`github.com/mattn/go-sqlite3`](https://github.com/mattn/go-sqlite3) for its local SQLite store, which requires CGO. The current release pipeline runs on a Linux runner and only produces Linux artifacts. macOS and Windows users build from source via `go install`.

## Quick reference

| OS | Command |
| --- | --- |
| Linux, WSL | `curl -fsSL https://raw.githubusercontent.com/Costroid/costroid-sync/main/install.sh \| sh` |
| Windows (PowerShell) | `irm https://raw.githubusercontent.com/Costroid/costroid-sync/main/install.ps1 \| iex` |
| macOS, any | `go install github.com/costroid/costroid-sync@latest` |

After install, run:

```sh
costroid-sync version
```

## Linux and WSL (prebuilt)

```sh
curl -fsSL https://raw.githubusercontent.com/Costroid/costroid-sync/main/install.sh | sh
```

Supported architectures: `amd64`, `arm64`. The `arm64` artifact is cross-compiled in CI but not exercised end-to-end on real arm64 hardware in CI — report any issues.

### Pin a release

```sh
curl -fsSL https://raw.githubusercontent.com/Costroid/costroid-sync/main/install.sh | VERSION=v0.2.0 sh
```

### Install location

Default: `/usr/local/bin/costroid-sync`. The script tries in order:

1. Copy directly if `/usr/local/bin` is writable.
2. Otherwise use `sudo` (you'll be prompted).
3. Otherwise print fallback steps for `~/.local/bin`.

Override with the `INSTALL_DIR` env var:

```sh
curl -fsSL https://raw.githubusercontent.com/Costroid/costroid-sync/main/install.sh | INSTALL_DIR="$HOME/.local/bin" sh
```

Make sure your chosen directory is on `PATH`.

### Checksum verification

The installer downloads `checksums.txt` alongside the tarball and verifies the SHA256 hash before extracting. It uses `sha256sum` if available, falls back to `shasum -a 256`, and prints a warning (without failing) if neither is present.

### WSL

WSL is just Linux. The installer detects WSL via `/proc/version` and prints `Detected WSL - using Linux build.`, then proceeds with the Linux path. Install into a WSL-side directory (default `/usr/local/bin`); do **not** install into a Windows-mounted path like `/mnt/c/...`.

## macOS (from source)

No prebuilt macOS binary yet. Install via Go:

```sh
xcode-select --install
go install github.com/costroid/costroid-sync@latest
```

### Prerequisites

- Go 1.24 or later — <https://go.dev/dl/>
- Xcode Command Line Tools — needed for `cc` so CGO can build `go-sqlite3`

### Where the binary lands

```sh
$(go env GOPATH)/bin/costroid-sync
```

Add that directory to your `PATH` if it isn't already:

```sh
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
```

### Pin a release

```sh
go install github.com/costroid/costroid-sync@v0.2.0
```

## Windows (from source via PowerShell)

```powershell
irm https://raw.githubusercontent.com/Costroid/costroid-sync/main/install.ps1 | iex
```

The installer:

1. Confirms `go` is on `PATH` (fails clearly if not).
2. Confirms `gcc` is on `PATH` (fails clearly if not — `go-sqlite3` won't link without it).
3. Runs `go install github.com/costroid/costroid-sync@latest`.
4. Reports the install path (`$(go env GOPATH)\bin\costroid-sync.exe`).
5. Prints a `PATH` update suggestion if needed.

### Prerequisites

- Go 1.24 or later — <https://go.dev/dl/>
- A C compiler: [MinGW-w64](https://www.mingw-w64.org/) or [msys2](https://www.msys2.org/). After install, make sure `gcc` is on your `PATH`.

### Pin a release

Download the script first, then run with `-Version`:

```powershell
irm https://raw.githubusercontent.com/Costroid/costroid-sync/main/install.ps1 -OutFile install.ps1
./install.ps1 -Version v0.2.0
```

Or via env var:

```powershell
$env:VERSION = "v0.2.0"
irm https://raw.githubusercontent.com/Costroid/costroid-sync/main/install.ps1 | iex
```

### Where the binary lands

```powershell
$(go env GOPATH)\bin\costroid-sync.exe
```

### Add it to your user PATH

```powershell
[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ';' + (Join-Path (& go env GOPATH) 'bin'), 'User')
```

This modifies your user `PATH` only and persists across sessions. Open a new PowerShell session afterward.

## From source (all OS)

If you already have a Go toolchain and a C compiler:

```sh
go install github.com/costroid/costroid-sync@latest
```

To build a local checkout:

```sh
git clone https://github.com/Costroid/costroid-sync
cd costroid-sync
go build -o costroid-sync .
./costroid-sync version
```

Requires Go 1.24+ and a working C compiler.

## Manual release download (Linux)

1. Open <https://github.com/Costroid/costroid-sync/releases> and pick a release.
2. Download `costroid-sync_linux_amd64.tar.gz` or `costroid-sync_linux_arm64.tar.gz`.
3. Download `checksums.txt` from the same release.
4. Verify:

   ```sh
   sha256sum -c checksums.txt --ignore-missing
   ```

5. Extract and place on `PATH`:

   ```sh
   tar -xzf costroid-sync_linux_amd64.tar.gz
   sudo install -m 0755 costroid-sync /usr/local/bin/
   ```

## Verify your install

```sh
costroid-sync version
```

You should see the release tag (e.g. `v0.2.0`).

## Troubleshooting

**`Permission denied` writing to `/usr/local/bin`.** Re-run with `sudo`, or set `INSTALL_DIR=$HOME/.local/bin` and make sure that directory is on your `PATH`.

**`command not found: costroid-sync` after install succeeded.** The install directory isn't on your `PATH`. For Linux/macOS, add it in your shell rc file. For Windows, see the `setx`-equivalent snippet above.

**`go: command not found`.** Install Go 1.24+ from <https://go.dev/dl/>, restart your shell, then re-run the installer.

**`exec: "gcc": executable file not found in $PATH`** (or `cgo: C compiler "gcc" not found`). The build needs a C compiler for `go-sqlite3`. macOS: `xcode-select --install`. Linux: `sudo apt install build-essential` (Debian/Ubuntu) or equivalent. Windows: install MinGW-w64 or msys2 and confirm `gcc --version` works in a fresh shell.

**`Checksum verification failed`.** Re-run the installer in case of a corrupted download. If it keeps failing, file an issue with the release tag and the expected/actual hashes the installer printed.

**Behind a corporate proxy.** Set `HTTPS_PROXY` (and `HTTP_PROXY` if needed) before running the installer. For `go install`, also set `GOPROXY` if your network blocks `proxy.golang.org`.

**Unsupported architecture warning on Linux.** Only `amd64` and `arm64` prebuilts are shipped. For other architectures, install from source.

## Privacy note

The install scripts only download release artifacts from GitHub and place a binary on your local filesystem. They do not contact any Costroid-operated service and do not transmit any user, system, or usage data. `costroid-sync` itself stores normalized cost metadata locally in `~/.costroid/costroid.db`; see the main [README](../README.md#security-and-privacy) for the full privacy model.
