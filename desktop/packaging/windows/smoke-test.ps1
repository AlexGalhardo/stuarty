<#
Installs the Windows desktop installer silently, checks what must hold (SDD-009 section 10.4), uninstalls
and checks the uninstall. The window itself is not started: a CI runner has no desktop to show it.

  smoke-test.ps1 -Installer <path of the setup .exe>

It uses a temporary install folder and a temporary workspace.
#>
param([Parameter(Mandatory = $true)][string]$Installer)
$ErrorActionPreference = 'Stop'

function Assert([bool]$Condition, [string]$Message) {
	if (-not $Condition) { throw "smoke test failed: $Message" }
	Write-Host "ok: $Message"
}

$root = Join-Path ([IO.Path]::GetTempPath()) ("stuarty-desktop-smoke-" + [guid]::NewGuid().ToString('N').Substring(0, 8))
$app = Join-Path $root 'app'
$workspace = Join-Path $root 'workspace'
New-Item -ItemType Directory -Force -Path $workspace | Out-Null
Set-Content -Path (Join-Path $workspace 'mine.txt') -Value 'keep me'
$lnk = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Stuarty Desktop.lnk'

try {
	$p = Start-Process -FilePath $Installer -Wait -PassThru -ArgumentList @(
		'/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/CURRENTUSER', "/DIR=$app", "/LOG=$root\setup.log")
	Assert ($p.ExitCode -eq 0) 'the installer exits 0'
	Assert (Test-Path (Join-Path $app 'stuarty-desktop.exe')) 'stuarty-desktop.exe is installed'
	Assert (Test-Path (Join-Path $app 'stuarty.exe')) 'stuarty.exe is installed next to it'
	Assert (Test-Path $lnk) 'the Start menu shortcut exists'

	$shortcut = (New-Object -ComObject WScript.Shell).CreateShortcut($lnk)
	Assert ($shortcut.TargetPath -eq (Join-Path $app 'stuarty-desktop.exe')) 'the shortcut targets stuarty-desktop.exe'

	$env:STUARTY_HOME = $workspace
	$version = & (Join-Path $app 'stuarty.exe') version
	Assert ($LASTEXITCODE -eq 0 -and $version) 'the installed server runs: stuarty version'

	# The application writes this value when the person turns on "Start when I sign in" (epic 21); the installer
	# never does. It is written here the way the application writes it, to prove the uninstaller removes it.
	$run = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
	$command = '"' + (Join-Path $app 'stuarty-desktop.exe') + '" --background'
	# A clean CI runner has no Run key. Never New-Item -Force here: on an existing key it erases every other value.
	if (-not (Test-Path $run)) { New-Item -Path $run | Out-Null }
	New-ItemProperty -Path $run -Name 'StuartyDesktop' -PropertyType String -Value $command -Force | Out-Null

	$u = Start-Process -FilePath (Join-Path $app 'unins000.exe') -Wait -PassThru -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART')
	Assert ($u.ExitCode -eq 0) 'the uninstaller exits 0'
	Start-Sleep -Seconds 2
	Assert (-not (Test-Path (Join-Path $app 'stuarty-desktop.exe'))) 'the program is removed'
	Assert (-not (Test-Path $lnk)) 'the shortcut is removed'
	$left = Get-ItemProperty -Path $run -Name 'StuartyDesktop' -ErrorAction SilentlyContinue
	Assert ($null -eq $left) 'the start at sign-in is removed'
	Assert (Test-Path (Join-Path $workspace 'mine.txt')) 'the workspace is kept'
}
finally {
	# Whatever happened above, the check leaves no start at sign-in behind on the machine it ran on.
	Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'StuartyDesktop' -ErrorAction SilentlyContinue
	Remove-Item Env:STUARTY_HOME -ErrorAction SilentlyContinue
	Get-Process stuarty, stuarty-desktop -ErrorAction SilentlyContinue | Stop-Process -Force
	Remove-Item -Recurse -Force $root -ErrorAction SilentlyContinue
}
Write-Host 'smoke test passed'
