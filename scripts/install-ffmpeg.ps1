# Installs the pinned LGPL FFmpeg of .config/ffmpeg.env on Windows (ADR-0011, SDD-009 11.2).
#
# Downloads the BtbN Windows asset by exact URL, verifies its SHA-256, unpacks it into a
# temporary folder and prints the bin folder as the last line of the output. When
# GITHUB_PATH is set (GitHub Actions) the bin folder is appended to it.
#
# As scripts/install-ffmpeg.sh, it never deletes an installed folder in place, because a
# running FFmpeg may hold it open (VV-227): a folder whose .archive-sha256 holds the pinned
# digest is reused without a download, and any other is replaced by unpacking beside it and
# swapping by rename. A swap that fails leaves the installed folder as it was.
#
# Environment: FFMPEG_ENV_FILE (default .config/ffmpeg.env of the repository),
# FFMPEG_INSTALL_DIR (default $env:RUNNER_TEMP or $env:TEMP, then ffmpeg under it).
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# The staging folder of an install in progress; Fail removes it, since exit skips no cleanup otherwise.
$stage = $null

function Fail([string]$Message) {
	if ($script:stage) { Remove-Item -LiteralPath $script:stage -Recurse -Force -ErrorAction SilentlyContinue }
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
$rootName = [IO.Path]::GetFileNameWithoutExtension($asset)
$marker = '.archive-sha256'

function Get-Sha256([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }

# Test-Tree: the folder holds a complete, LGPL FFmpeg tree; returns the banner.
function Test-Tree([string]$Dir) {
	$root = Join-Path $Dir $rootName
	$bin = Join-Path $root 'bin'
	if (-not (Test-Path -LiteralPath $bin)) { Fail "unexpected archive layout: $bin not found" }
	foreach ($tool in 'ffmpeg.exe', 'ffprobe.exe') {
		if (-not (Test-Path -LiteralPath (Join-Path $bin $tool))) { Fail "$tool is missing in $bin" }
	}
	if (-not (Test-Path -LiteralPath (Join-Path $root 'LICENSE.txt'))) { Fail "LICENSE.txt is missing in $root" }
	$text = (& (Join-Path $bin 'ffmpeg.exe') -version) -join "`n"
	if ($text -match '--enable-gpl|--enable-nonfree') { Fail 'the build is not LGPL: its configure line has --enable-gpl or --enable-nonfree' }
	return $text
}

# Test-Installed: the installed folder was unpacked from this archive and is complete.
function Test-Installed {
	$m = Join-Path $outDir $marker
	if (-not (Test-Path -LiteralPath $m -PathType Leaf)) { return $false }
	if ((Get-Content -LiteralPath $m -Raw).Trim() -ne $expected) { return $false }
	foreach ($f in 'bin\ffmpeg.exe', 'bin\ffprobe.exe', 'LICENSE.txt') {
		if (-not (Test-Path -LiteralPath (Join-Path (Join-Path $outDir $rootName) $f) -PathType Leaf)) { return $false }
	}
	return $true
}

$reuse = Test-Installed
if ($reuse) {
	[Console]::Error.WriteLine("install-ffmpeg: reusing the installed folder $outDir (archive digest matches)")
} elseif ((Test-Path -LiteralPath $archive) -and ((Get-Sha256 $archive) -eq $expected)) {
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

if (-not $reuse) {
	# Unpack beside the target and swap by rename, so a failure or an FFmpeg in use never leaves a
	# half-written install.
	$stage = Join-Path $base ".$tag-WINDOWS_AMD64.new.$PID"
	if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
	New-Item -ItemType Directory -Force -Path $stage | Out-Null
	try { Expand-Archive -LiteralPath $archive -DestinationPath $stage -Force }
	catch { Fail "could not unpack ${archive}: $($_.Exception.Message)" }
	$null = Test-Tree $stage
	# LF and no byte order mark, so install-ffmpeg.sh reads the same digest.
	[IO.File]::WriteAllText((Join-Path $stage $marker), "$expected`n")
	$old = $null
	if (Test-Path -LiteralPath $outDir) {
		$old = Join-Path $base ".$tag-WINDOWS_AMD64.old.$PID"
		try { Move-Item -LiteralPath $outDir -Destination $old }
		catch { Fail "cannot replace $outDir (is FFmpeg running from it?); it was left untouched" }
	}
	try { Move-Item -LiteralPath $stage -Destination $outDir }
	catch {
		if ($old) {
			try { Move-Item -LiteralPath $old -Destination $outDir }
			catch { Fail "could not move the new install into $outDir, nor the old one back from $old" }
		}
		Fail "could not move the new install into $outDir; the installed folder was left as it was"
	}
	$stage = $null
	if ($old) {
		try { Remove-Item -LiteralPath $old -Recurse -Force }
		catch { [Console]::Error.WriteLine("install-ffmpeg: could not remove the old folder $old; delete it by hand") }
	}
}

$root = Join-Path $outDir $rootName
$binDir = Join-Path $root 'bin'
$banner = Test-Tree $outDir
[Console]::Error.WriteLine(($banner -split "`n")[0])

if ($env:GITHUB_PATH) { Add-Content -LiteralPath $env:GITHUB_PATH -Value $binDir }
Write-Output $binDir
