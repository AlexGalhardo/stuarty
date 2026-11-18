<#
Installs the Windows installer silently, uses the shortcut, uninstalls, and checks what must hold
(SDD-009 section 10.4): the program and the Start menu shortcut exist, the shortcut runs
"vukem-video open", which starts the local server (VUKEM_VIDEO_NO_BROWSER keeps the browser closed),
the PATH has the program, and the uninstall removes all of it, stops the server and keeps the workspace.

  smoke-test.ps1 -Installer <path of the setup .exe>

It uses a temporary install folder and a temporary workspace, and restores the user PATH.
#>
param([Parameter(Mandatory = $true)][string]$Installer)
$ErrorActionPreference = 'Stop'

function Assert([bool]$Condition, [string]$Message) {
	if (-not $Condition) { throw "smoke test failed: $Message" }
	Write-Host "ok: $Message"
}

$root = Join-Path ([IO.Path]::GetTempPath()) ("vukem-video-smoke-" + [guid]::NewGuid().ToString('N').Substring(0, 8))
$app = Join-Path $root 'app'
$workspace = Join-Path $root 'workspace'
New-Item -ItemType Directory -Force -Path $workspace | Out-Null
Set-Content -Path (Join-Path $workspace 'mine.txt') -Value 'keep me'
$lnk = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Vukem Video.lnk'
$pathBefore = [Environment]::GetEnvironmentVariable('Path', 'User')

try {
	$p = Start-Process -FilePath $Installer -Wait -PassThru -ArgumentList @(
		'/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/CURRENTUSER', "/DIR=$app", '/TASKS=addtopath', "/LOG=$root\setup.log")
	Assert ($p.ExitCode -eq 0) 'the installer exits 0'
	Assert (Test-Path (Join-Path $app 'vukem-video.exe')) 'vukem-video.exe is installed'
	Assert (Test-Path $lnk) 'the Start menu shortcut exists'

	$shortcut = (New-Object -ComObject WScript.Shell).CreateShortcut($lnk)
	Assert ($shortcut.TargetPath -eq (Join-Path $app 'vukem-video.exe')) 'the shortcut targets vukem-video.exe'
	Assert ($shortcut.Arguments -eq 'open') 'the shortcut runs "vukem-video open"'
	Assert ([Environment]::GetEnvironmentVariable('Path', 'User') -like "*$app*") 'the program is on the user PATH'

	# Use the shortcut as a user would, with the browser kept closed and a workspace of our own.
	$env:VUKEM_VIDEO_HOME = $workspace
	$env:VUKEM_VIDEO_NO_BROWSER = '1'
	Start-Process -FilePath $lnk
	$up = $false
	for ($i = 0; $i -lt 60 -and -not $up; $i++) {
		Start-Sleep -Milliseconds 500
		try { $up = (Invoke-RestMethod -Uri 'http://127.0.0.1:7878/up' -TimeoutSec 2).status -eq 'ok' } catch { $up = $false }
	}
	Assert $up 'the shortcut started the local server: GET /up answers ok'

	$u = Start-Process -FilePath (Join-Path $app 'unins000.exe') -Wait -PassThru -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART')
	Assert ($u.ExitCode -eq 0) 'the uninstaller exits 0'
	Start-Sleep -Seconds 2
	Assert (-not (Test-Path (Join-Path $app 'vukem-video.exe'))) 'the program is removed'
	Assert (-not (Test-Path $lnk)) 'the shortcut is removed'
	Assert (-not ([Environment]::GetEnvironmentVariable('Path', 'User') -like "*$app*")) 'the program is off the user PATH'
	Assert (-not (Get-Process vukem-video -ErrorAction SilentlyContinue)) 'the background server is stopped'
	Assert ((Test-Path (Join-Path $workspace 'mine.txt')) -and (Test-Path (Join-Path $workspace 'app.key'))) 'the workspace is kept'
}
finally {
	Remove-Item Env:VUKEM_VIDEO_HOME, Env:VUKEM_VIDEO_NO_BROWSER -ErrorAction SilentlyContinue
	Get-Process vukem-video -ErrorAction SilentlyContinue | Stop-Process -Force
	if ($pathBefore -ne [Environment]::GetEnvironmentVariable('Path', 'User')) {
		[Environment]::SetEnvironmentVariable('Path', $pathBefore, 'User')
	}
	Remove-Item -Recurse -Force $root -ErrorAction SilentlyContinue
}
Write-Host 'smoke test passed'
