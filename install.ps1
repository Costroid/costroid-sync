#!/usr/bin/env pwsh
# costroid-sync PowerShell installer.
# Builds from source via 'go install' - no prebuilt Windows binary yet.

[CmdletBinding()]
param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"

$ModulePath = "github.com/costroid/costroid-sync"
$Repo = "Costroid/costroid-sync"
$DocsUrl = "https://github.com/$Repo/blob/main/docs/install.md"

if ([string]::IsNullOrEmpty($Version)) {
    if ($env:VERSION) {
        $Version = $env:VERSION
    } else {
        $Version = "latest"
    }
}

function Write-Err {
    param([string]$Message)
    [Console]::Error.WriteLine($Message)
}

Write-Output "Building costroid-sync from source via 'go install' (no prebuilt Windows binary yet)."
Write-Output ""

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Err "Go 1.24+ is required."
    Write-Err "Install Go from: https://go.dev/dl/"
    Write-Err "Then re-run this installer."
    Write-Err "Details: $DocsUrl"
    exit 1
}

if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    Write-Err "A C compiler is required because go-sqlite3 uses CGO."
    Write-Err "Install MinGW-w64 (https://www.mingw-w64.org/) or msys2 (https://www.msys2.org/),"
    Write-Err "ensure 'gcc' is on your PATH, then re-run this installer."
    Write-Err "Details: $DocsUrl"
    exit 1
}

$arch = $env:PROCESSOR_ARCHITECTURE
switch ($arch) {
    "AMD64" { $archDisplay = "amd64" }
    "ARM64" { $archDisplay = "arm64" }
    default { $archDisplay = $arch }
}
Write-Output "Detected Windows / $archDisplay."

if ($Version -eq "latest") {
    $target = "$ModulePath@latest"
} else {
    $target = "$ModulePath@$Version"
}

Write-Output "Running: go install $target"
& go install $target
if ($LASTEXITCODE -ne 0) {
    Write-Err ""
    Write-Err "go install failed (exit code $LASTEXITCODE)."
    Write-Err "Common causes:"
    Write-Err "  - C compiler not on PATH (re-check MinGW-w64 / msys2 install)"
    Write-Err "  - Network or GitHub access blocked"
    Write-Err "  - Invalid version tag (try -Version latest)"
    Write-Err "Details: $DocsUrl"
    exit $LASTEXITCODE
}

$gopath = (& go env GOPATH).Trim()
$binDir = Join-Path $gopath "bin"
$installedBin = Join-Path $binDir "costroid-sync.exe"

Write-Output ""
Write-Output "Installed: $installedBin"

$pathEntries = $env:PATH -split ";"
$onPath = $false
foreach ($entry in $pathEntries) {
    if ($entry.TrimEnd("\") -ieq $binDir.TrimEnd("\")) {
        $onPath = $true
        break
    }
}

if (-not $onPath) {
    Write-Output ""
    Write-Output "WARNING: $binDir is not on your PATH."
    Write-Output "Add it to your user PATH (no admin required):"
    Write-Output ""
    Write-Output "  [Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ';$binDir', 'User')"
    Write-Output ""
    Write-Output "Then open a new PowerShell session and run: costroid-sync version"
} else {
    Write-Output ""
    Write-Output "Run: costroid-sync version"
}
