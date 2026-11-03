# Opt-in integration test against an already installed widget. Leaves it running.
. "$PSScriptRoot\Core.ps1"
$root=Get-WidgetRoot
function Get-HelperProcesses {
    @(Get-CimInstance Win32_Process -Filter "Name='powershell.exe'" | Where-Object { $_.CommandLine -like ('*'+$root+'\Helper.ps1*') })
}
function Wait-HelperCount([int]$Count) {
    for ($i=0; $i -lt 100; $i++) {
        if (@(Get-HelperProcesses).Count -eq $Count) { return }
        [Threading.Thread]::Sleep(100)
    }
    throw 'Unexpected helper process count'
}
& "$root\Start.ps1"
& "$root\Start.ps1"
& "$root\Start.ps1"
Wait-HelperCount 1
# A DPAPI-loading failure can terminate shortly after the process first appears.
for ($i=0; $i -lt 100; $i++) {
    $text=Get-Content -Raw -LiteralPath (Join-Path $root 'snapshot.ini') -ErrorAction SilentlyContinue
    $match=[regex]::Match($text,'(?m)^Written=(\d+)')
    if ($match.Success -and [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()-[long]$match.Groups[1].Value -lt 20 -and $text -match '(?m)^Connection=Connected\r?$') { break }
    [Threading.Thread]::Sleep(100)
}
if ($i -eq 100) { throw 'Hidden helper did not produce a fresh connected snapshot' }
Wait-HelperCount 1
Write-Output 'PASS: hidden helper and singleton'
$processes=Get-HelperProcesses
& "$root\Stop.ps1"
foreach ($proc in $processes) { Wait-Process -Id $proc.ProcessId -Timeout 10 -ErrorAction Stop }
Wait-HelperCount 0
Write-Output 'PASS: installed helper stopped'
& "$root\Start.ps1"
Wait-HelperCount 1
for ($i=0; $i -lt 100; $i++) {
    $text=Get-Content -Raw -LiteralPath (Join-Path $root 'snapshot.ini') -ErrorAction SilentlyContinue
    if ($text -match '(?m)^Connection=Connected\r?$') { break }
    [Threading.Thread]::Sleep(100)
}
if ($i -eq 100) { throw 'Restarted helper did not connect' }
Write-Output 'PASS: hidden helper restarted and connected'
if (Test-Path (Join-Path ([Environment]::GetFolderPath('Startup')) 'Start CLIProxyAccounts.lnk')) { Write-Output 'Startup shortcut present' } else { Write-Output 'PASS: no default startup mutation' }
