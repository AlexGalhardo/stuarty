# Measures NFR-067 (PRD-004 section 5): a window of 1920 by 1080 recorded at 60 fps for 10 minutes loses at most 1%
# of the frames. One command; it opens the test window of its own, records it through the real capture helper, the
# real host and the real FFmpeg, prints the dropped frames and the load of the machine, and leaves nothing running.
#
#   powershell -File scripts/measure-capture.ps1 [-Seconds 600] [-FfmpegDir <dir of the pinned ffmpeg.exe>]
#                                               [-Helper <stuarty-capture.exe>] [-Left <x of the test window>]
#
# The number counts only on an idle machine: stop every other agent and program first. The script prints the average
# load of the machine during the run so that a reader can see whether it was idle.
param(
	[int] $Seconds = 600,
	[string] $FfmpegDir = $env:STUARTY_TEST_FFMPEG_DIR,
	[string] $Helper = '',
	[int] $Left = -1
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

if (-not $FfmpegDir) { throw 'give -FfmpegDir or set STUARTY_TEST_FFMPEG_DIR: the folder of the pinned LGPL ffmpeg.exe' }

if (-not $Helper) {
	Write-Host 'building the capture helper'
	& bash (Join-Path $root 'scripts/build-capture.sh')
	if ($LASTEXITCODE -ne 0) { throw 'the capture helper did not build' }
	$Helper = Join-Path $root 'capture/dist/stuarty-capture.exe'
}

Add-Type -AssemblyName System.Windows.Forms
if ($Left -lt 0) { $Left = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Width - 1 }

$title = 'stuarty-nfr067-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
$window = $null
$sampler = $null
try {
	# One pixel of the window must lie on a display, or Windows sends it no frame (spikes/capture-windows).
	$window = Start-Process -FilePath powershell -PassThru -WindowStyle Hidden -ArgumentList @(
		'-NoProfile', '-File', (Join-Path $PSScriptRoot 'measure-capture-window.ps1'), '-Title', $title, '-Left', $Left)
	Start-Sleep -Seconds 3

	$samples = Join-Path ([IO.Path]::GetTempPath()) ($title + '.load')
	$sampler = Start-Job -ArgumentList $samples -ScriptBlock {
		param($file)
		while ($true) {
			# The WMI class has the same name in every language of Windows, unlike the names of the counters.
			(Get-CimInstance Win32_PerfFormattedData_PerfOS_Processor -Filter "Name='_Total'").PercentProcessorTime | Out-File -Append -Encoding ascii $file
			Start-Sleep -Seconds 5
		}
	}

	$env:STUARTY_BENCH_WINDOW = $title
	$env:STUARTY_BENCH_SECONDS = "$Seconds"
	$env:STUARTY_TEST_CAPTURE_HELPER = (Resolve-Path $Helper).Path
	$env:STUARTY_TEST_FFMPEG_DIR = (Resolve-Path $FfmpegDir).Path

	Push-Location $root
	try {
		& go test -count=1 -v -timeout ("{0}s" -f ($Seconds + 900)) -run TestCaptureNFR067 ./web/app/servicetest/ 2>&1 |
			Where-Object { $_ -notmatch '^time=' } | ForEach-Object { Write-Host $_ }
		$status = $LASTEXITCODE
	}
	finally { Pop-Location }

	if (Test-Path $samples) {
		$values = Get-Content $samples | ForEach-Object { [double]$_ }
		$avg = ($values | Measure-Object -Average).Average
		$max = ($values | Measure-Object -Maximum).Maximum
		Write-Host ('load of the machine during the run: average {0:N0}%, highest sample {1:N0}% ({2} samples)' -f $avg, $max, $values.Count)
		Remove-Item $samples -ErrorAction SilentlyContinue
	}
	exit $status
}
finally {
	if ($sampler) { Stop-Job $sampler -ErrorAction SilentlyContinue; Remove-Job $sampler -Force -ErrorAction SilentlyContinue }
	if ($window -and -not $window.HasExited) { Stop-Process -Id $window.Id -Force -ErrorAction SilentlyContinue }
}
