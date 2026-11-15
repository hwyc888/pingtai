$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

function Invoke-Go {
    param([string[]]$GoArgs)
    & go @GoArgs
    if ($LASTEXITCODE -ne 0) {
        throw "go $($GoArgs -join ' ') failed with exit code $LASTEXITCODE"
    }
}

$FrontendSource = Join-Path $Root "frontend"
$FrontendDist = Join-Path $FrontendSource "dist"
$EmbeddedWeb = Join-Path $Root "internal\httpapi\web"
if (!(Test-Path (Join-Path $FrontendSource "package.json"))) {
    throw "Frontend package.json not found: $FrontendSource"
}
if (!(Get-Command npm -ErrorAction SilentlyContinue)) {
    throw "Node.js and npm are required to build the Vue frontend. Install Node.js 20.19 or newer."
}

Push-Location $FrontendSource
try {
    if (Test-Path (Join-Path $FrontendSource "package-lock.json")) {
        & npm ci --no-audit --no-fund
    }
    else {
        & npm install --no-audit --no-fund
    }
    if ($LASTEXITCODE -ne 0) {
        throw "npm dependency installation failed with exit code $LASTEXITCODE"
    }

    & npm run build
    if ($LASTEXITCODE -ne 0) {
        throw "npm run build failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

if (!(Test-Path (Join-Path $FrontendDist "index.html"))) {
    throw "Frontend build output not found: $FrontendDist\index.html"
}

if (Test-Path $EmbeddedWeb) {
    Remove-Item -LiteralPath $EmbeddedWeb -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $EmbeddedWeb | Out-Null
Get-ChildItem -Force $FrontendDist |
    Copy-Item -Destination $EmbeddedWeb -Recurse -Force

Invoke-Go @("mod", "tidy")
Invoke-Go @("fmt", "./...")
Invoke-Go @("test", "./...")
Invoke-Go @("vet", "./...")

$Dist = Join-Path $Root "dist"
$DeployDist = Join-Path $Dist "deploy"
New-Item -ItemType Directory -Force -Path $Dist | Out-Null

# Remove the old external frontend copy: the new executables serve embedded assets.
$OldFrontendDist = Join-Path $Dist "frontend"
if (Test-Path $OldFrontendDist) {
    Remove-Item -LiteralPath $OldFrontendDist -Recurse -Force
}
if (Test-Path $DeployDist) {
    Remove-Item -LiteralPath $DeployDist -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $DeployDist | Out-Null

Invoke-Go @("build", "-trimpath", "-ldflags=-s -w", "-o", (Join-Path $Dist "pingtai.exe"), "./cmd/pingtai")

$OldGOOS = $env:GOOS
$OldGOARCH = $env:GOARCH
$OldCGO = $env:CGO_ENABLED
try {
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    Invoke-Go @("build", "-trimpath", "-ldflags=-s -w", "-o", (Join-Path $Dist "pingtai-linux-amd64"), "./cmd/pingtai")
}
finally {
    $env:GOOS = $OldGOOS
    $env:GOARCH = $OldGOARCH
    $env:CGO_ENABLED = $OldCGO
}

Copy-Item -Path (Join-Path $Root ".env.example") -Destination (Join-Path $Dist ".env.example") -Force
Copy-Item -Path (Join-Path $Root "README_GO.md") -Destination (Join-Path $Dist "README.md") -Force
Copy-Item -Path (Join-Path $Root "manage-go.sh") -Destination (Join-Path $Dist "manage-go.sh") -Force
Copy-Item -Path (Join-Path $Root "start.bat") -Destination (Join-Path $Dist "start.bat") -Force
Get-ChildItem -Force (Join-Path $Root "deploy") |
    Copy-Item -Destination $DeployDist -Recurse -Force

# Build a clean staging folder so local .env files and secrets are never included.
$Staging = Join-Path $Dist "_zip_staging"
if (Test-Path $Staging) {
    Remove-Item -LiteralPath $Staging -Recurse -Force
}
New-Item -ItemType Directory -Force -Path (Join-Path $Staging "deploy") | Out-Null

$PackageFiles = @(
    "pingtai.exe",
    "pingtai-linux-amd64",
    ".env.example",
    "README.md",
    "start.bat",
    "manage-go.sh"
)
foreach ($Name in $PackageFiles) {
    Copy-Item -LiteralPath (Join-Path $Dist $Name) -Destination $Staging -Force
}
Get-ChildItem -Force $DeployDist |
    Copy-Item -Destination (Join-Path $Staging "deploy") -Recurse -Force

$ZipPath = Join-Path $Dist "pingtai-release.zip"
if (Test-Path $ZipPath) {
    Remove-Item -LiteralPath $ZipPath -Force
}
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$Archive = [System.IO.Compression.ZipFile]::Open(
    $ZipPath,
    [System.IO.Compression.ZipArchiveMode]::Create
)
try {
    Get-ChildItem -LiteralPath $Staging -Recurse -File | ForEach-Object {
        $EntryName = $_.FullName.Substring($Staging.Length).TrimStart([char[]]@("\", "/")).Replace("\", "/")
        [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile(
            $Archive,
            $_.FullName,
            $EntryName,
            [System.IO.Compression.CompressionLevel]::Optimal
        ) | Out-Null
    }
}
finally {
    $Archive.Dispose()
}
Remove-Item -LiteralPath $Staging -Recurse -Force

Write-Host ""
Write-Host "Release built successfully:"
Get-ChildItem -Force $Dist | Select-Object Name, Length
Write-Host ""
Write-Host "ZIP: $ZipPath"
