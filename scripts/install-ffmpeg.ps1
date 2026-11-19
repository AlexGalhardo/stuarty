# Installs the pinned LGPL FFmpeg of .config/ffmpeg.env on Windows (ADR-0011, SDD-009 11.2).
#
# Downloads the BtbN Windows asset by exact URL, verifies its SHA-256, unpacks it into a
# temporary folder and prints the bin folder as the last line of the output. When
# GITHUB_PATH is set (GitHub Actions) the bin folder is appended to it.
#
# Environment: FFMPEG_ENV_FILE (default .config/ffmpeg.env of the repository),
# FFMPEG_INSTALL_DIR (default $env:RUNNER_TEMP or $env:TEMP, then ffmpeg under it).
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Fail([string]$Message) {
	[Console]::Error.WriteLine("install-ffmpeg: error: $Message")
	exit 1
}

$envFile = $env:FFMPEG_ENV_FILE
if (-not $envFile) { $envFile = Join-Path $PSScriptRoot '..\.config\ffmpeg.env' }
if (-not (Test-Path -LiteralPath $envFile)) { Fail "pin file not found: $envFile" }

# The file is a bash fragment of KEY="value" lines; read it without running it.
$pin = @{}
foreach ($line in Get-Content -LiteralPath $envFile) {
	if ($line -match '^\s*([A-Z0-9_]+)="([^"]*)"\s*$') { $pin[$Matches[1]] = $Matches[2] }
}
foreach ($k in 'FFMPEG_RELEASE_TAG', 'FFMPEG_VERSION', 'FFMPEG_ASSET_WINDOWS_AMD64', 'FFMPEG_SHA256_WINDOWS_AMD64') {
	if (-not $pin[$k]) { Fail "$k is missing in $envFile" }
}
$tag = $pin['FFMPEG_RELEASE_TAG']
$asset = $pin['FFMPEG_ASSET_WINDOWS_AMD64']
$expected = $pin['FFMPEG_SHA256_WINDOWS_AMD64']
if ($expected -cnotmatch '^[0-9a-f]{64}$') { Fail "FFMPEG_SHA256_WINDOWS_AMD64 is not 64 lowercase hex characters in $envFile" }

$base = $env:FFMPEG_INSTALL_DIR
if (-not $base) {
	$tmp = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { $env:TEMP }
	$base = Join-Path $tmp 'ffmpeg'
}
$dlDir = Join-Path $base "download\$tag"
$outDir = Join-Path $base "$tag-WINDOWS_AMD64"
New-Item -ItemType Directory -Force -Path $dlDir | Out-Null
$archive = Join-Path $dlDir $asset
$url = "https://github.com/BtbN/FFmpeg-Builds/releases/download/$tag/$asset"

function Get-Sha256([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }

if ((Test-Path -LiteralPath $archive) -and ((Get-Sha256 $archive) -eq $expected)) {
	[Console]::Error.WriteLine("install-ffmpeg: reusing the verified download $archive")
} else {
	[Console]::Error.WriteLine("install-ffmpeg: downloading $url")
	$ProgressPreference = 'SilentlyContinue'
	try { Invoke-WebRequest -Uri $url -OutFile $archive -UseBasicParsing }
	catch { Fail "download failed: $url ($($_.Exception.Message))" }
	$actual = Get-Sha256 $archive
	if ($actual -ne $expected) {
		Remove-Item -LiteralPath $archive -Force
		Fail "SHA-256 mismatch for ${asset}: expected $expected, got $actual. The download was deleted and nothing was unpacked."
	}
	[Console]::Error.WriteLine("install-ffmpeg: SHA-256 verified ($expected)")
}

if (Test-Path -LiteralPath $outDir) { Remove-Item -LiteralPath $outDir -Recurse -Force }
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
Expand-Archive -LiteralPath $archive -DestinationPath $outDir -Force

$root = Join-Path $outDir ([IO.Path]::GetFileNameWithoutExtension($asset))
$binDir = Join-Path $root 'bin'
if (-not (Test-Path -LiteralPath $binDir)) { Fail "unexpected archive layout: $binDir not found" }
foreach ($tool in 'ffmpeg.exe', 'ffprobe.exe') {
	if (-not (Test-Path -LiteralPath (Join-Path $binDir $tool))) { Fail "$tool is missing in $binDir" }
}
if (-not (Test-Path -LiteralPath (Join-Path $root 'LICENSE.txt'))) { Fail "LICENSE.txt is missing in $root" }

$banner = (& (Join-Path $binDir 'ffmpeg.exe') -version) -join "`n"
if ($banner -match '--enable-gpl|--enable-nonfree') { Fail 'the build is not LGPL: its configure line has --enable-gpl or --enable-nonfree' }
[Console]::Error.WriteLine(($banner -split "`n")[0])

if ($env:GITHUB_PATH) { Add-Content -LiteralPath $env:GITHUB_PATH -Value $binDir }
Write-Output $binDir
