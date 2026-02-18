#Requires -Version 5.1
<#
.SYNOPSIS
    Install oficina agent (or server) on Windows.

.DESCRIPTION
    Installs the oficina binary and optionally registers it as a Windows service
    (via NSSM) or a startup scheduled task.

.EXAMPLE
    .\install.ps1 -Server http://host:8080 -Name win-agent
    .\install.ps1 -Server http://host:8080 -Name win-agent -Service nssm
    .\install.ps1 -Server http://host:8080 -Name win-agent -Service schtasks
    .\install.ps1 -Component server -Service nssm

.PARAMETER Server
    The oficina server URL (required for agent).

.PARAMETER Name
    Agent name. Defaults to the hostname.

.PARAMETER Labels
    Comma-separated agent labels.

.PARAMETER Component
    Which component to install: agent (default) or server.

.PARAMETER InstallDir
    Where to place the binary. Default: C:\oficina

.PARAMETER Service
    Service install method: none (default), nssm, or schtasks.
#>

param(
    [string]$Server = "",
    [string]$Name = "",
    [string]$Labels = "",
    [ValidateSet("agent", "server")]
    [string]$Component = "agent",
    [string]$InstallDir = "C:\oficina",
    [ValidateSet("none", "nssm", "schtasks")]
    [string]$Service = "none"
)

$ErrorActionPreference = "Stop"

# ── Validate ──────────────────────────────────────────────
if ($Component -eq "agent" -and -not $Server) {
    Write-Error "The -Server parameter is required for agent install."
    exit 1
}

if ($Component -eq "agent" -and -not $Name) {
    $Name = $env:COMPUTERNAME
    Write-Host "No -Name given, using hostname: $Name"
}

# ── Detect arch ───────────────────────────────────────────
$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
$BinaryName = "oficina-$Component-windows-$Arch.exe"
$TargetName = "oficina-$Component.exe"

Write-Host "Platform: windows/$Arch"
Write-Host "Component: $Component"

# ── Find or build binary ─────────────────────────────────
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoDir = Split-Path -Parent $ScriptDir
$Binary = $null

# Check for pre-built binary
$Candidates = @(
    (Join-Path $ScriptDir $BinaryName),
    (Join-Path $ScriptDir "..\dist\$BinaryName"),
    (Join-Path $RepoDir "dist\$BinaryName")
)

foreach ($c in $Candidates) {
    if (Test-Path $c) {
        $Binary = (Resolve-Path $c).Path
        Write-Host "Found pre-built binary: $Binary"
        break
    }
}

# Build from source if no binary found
if (-not $Binary) {
    $GoCmd = Get-Command go -ErrorAction SilentlyContinue
    if (-not $GoCmd) {
        Write-Error "No pre-built binary found and Go is not installed. Build first with 'just release' or install Go."
        exit 1
    }
    $GoMod = Join-Path $RepoDir "go.mod"
    if (-not (Test-Path $GoMod)) {
        Write-Error "Not in the oficina repo and no pre-built binary found."
        exit 1
    }
    Write-Host "Building from source..."
    Push-Location $RepoDir
    try {
        $distDir = Join-Path $RepoDir "dist"
        if (-not (Test-Path $distDir)) { New-Item -ItemType Directory -Path $distDir | Out-Null }
        $env:CGO_ENABLED = "0"
        & go build -trimpath -ldflags="-s -w" -o "dist\$BinaryName" "./cmd/$Component"
        if ($LASTEXITCODE -ne 0) { throw "Build failed" }
        $Binary = Join-Path $distDir $BinaryName
        Write-Host "Built: $Binary"
    }
    finally {
        Pop-Location
    }
}

# ── Install binary ────────────────────────────────────────
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$TargetPath = Join-Path $InstallDir $TargetName
Copy-Item $Binary $TargetPath -Force
Write-Host "Installed: $TargetPath"

# ── Build args ────────────────────────────────────────────
$AgentArgs = @()
if ($Component -eq "agent") {
    $AgentArgs += "--server", $Server, "--name", $Name
    if ($Labels) {
        $AgentArgs += "--labels", $Labels
    }
}

# ── Service setup ─────────────────────────────────────────
if ($Service -eq "none") {
    Write-Host ""
    Write-Host "Done. Run with -Service nssm or -Service schtasks to install as a service."
    Write-Host ""
    Write-Host "To run manually:"
    Write-Host "  $TargetPath $($AgentArgs -join ' ')"
    exit 0
}

$ServiceName = "oficina-$Component"

if ($Service -eq "nssm") {
    # ── NSSM service ──────────────────────────────────────
    $NssmCmd = Get-Command nssm -ErrorAction SilentlyContinue
    if (-not $NssmCmd) {
        Write-Error "NSSM not found. Install it from https://nssm.cc/ or use -Service schtasks instead."
        exit 1
    }

    # Remove existing service if present
    & nssm status $ServiceName 2>$null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Stopping existing service..."
        & nssm stop $ServiceName 2>$null
        & nssm remove $ServiceName confirm
    }

    $ArgsString = $AgentArgs -join " "
    & nssm install $ServiceName $TargetPath $ArgsString
    & nssm set $ServiceName AppStdout (Join-Path $InstallDir "$ServiceName.log")
    & nssm set $ServiceName AppStderr (Join-Path $InstallDir "$ServiceName.log")
    & nssm set $ServiceName AppRotateFiles 1
    & nssm start $ServiceName

    Write-Host ""
    Write-Host "Installed NSSM service: $ServiceName"
    Write-Host "Logs: $(Join-Path $InstallDir "$ServiceName.log")"
    Write-Host ""
    Write-Host "Commands:"
    Write-Host "  nssm status  $ServiceName"
    Write-Host "  nssm restart $ServiceName"
    Write-Host "  nssm stop    $ServiceName"
    Write-Host "  nssm remove  $ServiceName confirm"
}
elseif ($Service -eq "schtasks") {
    # ── Scheduled task (runs at logon) ────────────────────
    $TaskExists = schtasks /query /tn $ServiceName 2>$null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Removing existing scheduled task..."
        schtasks /delete /tn $ServiceName /f
    }

    $ArgsString = $AgentArgs -join " "
    schtasks /create /tn $ServiceName /tr "`"$TargetPath`" $ArgsString" /sc onlogon /rl highest /f
    if ($LASTEXITCODE -ne 0) { throw "Failed to create scheduled task" }

    Write-Host ""
    Write-Host "Installed scheduled task: $ServiceName (runs at logon)"
    Write-Host ""
    Write-Host "Commands:"
    Write-Host "  schtasks /run    /tn $ServiceName"
    Write-Host "  schtasks /end    /tn $ServiceName"
    Write-Host "  schtasks /query  /tn $ServiceName"
    Write-Host "  schtasks /delete /tn $ServiceName /f"
}
