# Starts the spike, kills it once after $KillAfter seconds, and prints the wall time of the kill.
#   -Mode tree    kills the helper and its FFmpeg child at once (a crash of the whole tree)
#   -Mode helper  kills only the helper; FFmpeg sees the end of its input and finishes by itself
param(
	[Parameter(Mandatory)] [string] $Spike,
	[Parameter(Mandatory)] [string] $Ffmpeg,
	[Parameter(Mandatory)] [string] $OutDir,
	[int] $KillAfter = 300,
	[int] $Seconds = 600,
	[ValidateSet('tree', 'helper')] [string] $Mode = 'tree',
	[string] $Encoder = 'h264_nvenc',
	[string] $Name = 'killed',
	[string] $FlushPackets = '',
	[string] $FfmpegExtra = ''
)
$Extra = @()
if ($FlushPackets -ne '') { $Extra += @('--flush-packets', $FlushPackets) }
if ($FfmpegExtra -ne '') { $Extra += @('--extra', "`"$FfmpegExtra`"") }
$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$out = Join-Path $OutDir "$Name.mp4"
$log = Join-Path $OutDir "$Name.log"
$p = Start-Process -FilePath $Spike -PassThru -WindowStyle Hidden `
	-RedirectStandardOutput (Join-Path $OutDir "$Name.stdout") `
	-ArgumentList (@('--out', $out, '--seconds', $Seconds, '--ffmpeg', $Ffmpeg, '--log', $log, '--encoder', $Encoder, '--pos', '2559,0') + $Extra)
$sw = [Diagnostics.Stopwatch]::StartNew()
Start-Sleep -Seconds $KillAfter
$wall = $sw.Elapsed.TotalSeconds
if ($Mode -eq 'tree') { & taskkill /PID $p.Id /T /F | Out-Null } else { Stop-Process -Id $p.Id -Force }
"killed mode=$Mode helper_pid=$($p.Id) wall_s_since_spawn=$([math]::Round($wall, 2))" | Tee-Object -FilePath (Join-Path $OutDir "$Name.kill") | Write-Output
