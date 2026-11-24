# The test window of scripts/measure-capture.ps1: a borderless 1920 by 1080 window of its own that paints a
# scrolling pattern at every refresh, so the capture has a source that changes at least 60 times a second. It
# captures nothing and shows nothing of anything else.
param(
	[Parameter(Mandatory)] [string] $Title,
	[int] $Left = 0,
	[int] $Top = 0
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
Add-Type -TypeDefinition @'
using System.Runtime.InteropServices;
public static class NativeDpi {
	[DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
	[DllImport("dwmapi.dll")] public static extern int DwmFlush();
}
'@
[void][NativeDpi]::SetProcessDPIAware()

$stripe = 30
$period = $stripe * 24
$width = 1920
$height = 1080

# The pattern is drawn once into a bitmap one period wider than the window; one blit per frame scrolls it.
# A premultiplied bitmap is drawn to the screen without a conversion, which a plain ARGB one is not.
$bitmap = New-Object System.Drawing.Bitmap ($width + $period), $height, ([System.Drawing.Imaging.PixelFormat]::Format32bppPArgb)
$g = [System.Drawing.Graphics]::FromImage($bitmap)
for ($i = 0; $i -le (($width + $period) / $stripe); $i++) {
	$hue = ($i % 24) / 24.0
	$color = [System.Drawing.Color]::FromArgb(255, [int](255 * (0.5 + 0.5 * [math]::Sin(6.283 * $hue))), [int](255 * (0.5 + 0.5 * [math]::Sin(6.283 * ($hue + 0.33)))), [int](255 * (0.5 + 0.5 * [math]::Sin(6.283 * ($hue + 0.66)))))
	$brush = New-Object System.Drawing.SolidBrush $color
	$g.FillRectangle($brush, $i * $stripe, 0, $stripe, $height)
	$brush.Dispose()
}
$g.Dispose()

$form = New-Object System.Windows.Forms.Form
$form.Text = $Title
$form.FormBorderStyle = 'None'
$form.StartPosition = 'Manual'
$form.Location = New-Object System.Drawing.Point $Left, $Top
$form.ClientSize = New-Object System.Drawing.Size $width, $height
$form.ShowInTaskbar = $false
$form.TopMost = $false

$clock = [System.Diagnostics.Stopwatch]::StartNew()
$panel = New-Object System.Windows.Forms.Panel
$panel.Dock = 'Fill'
$panel.GetType().GetProperty('DoubleBuffered', [System.Reflection.BindingFlags]'Instance,NonPublic').SetValue($panel, $true, $null)
$panel.Add_Paint({
	param($sender, $e)
	$offset = [int]($clock.ElapsedMilliseconds * 6 / 10) % $period
	$e.Graphics.DrawImageUnscaled($bitmap, -$offset, 0)
})
$form.Controls.Add($panel)

# Paint once per refresh of the display: DwmFlush returns when the compositor has shown a frame.
$form.Show()
while ($form.Visible) {
	[System.Windows.Forms.Application]::DoEvents()
	$panel.Invalidate()
	$panel.Update()
	[void][NativeDpi]::DwmFlush()
}
