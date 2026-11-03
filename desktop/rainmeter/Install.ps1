param(
    [Parameter(Mandatory=$true)][string]$BaseUrl,
    [string]$RainmeterDirectory = (Join-Path $env:LOCALAPPDATA 'Programs\MementoMori'),
    [Security.SecureString]$ManagementKey,
    [switch]$ImportExistingOraclerKey,
    [switch]$StartWithWindows
)
. "$PSScriptRoot\Core.ps1"
$base=Get-PrivateRoot $BaseUrl
$root=Get-WidgetRoot
$exe=Join-Path $RainmeterDirectory 'Rainmeter.exe'
$ini=Join-Path $RainmeterDirectory 'Rainmeter.ini'
if (!(Test-Path -LiteralPath $exe) -or !(Test-Path -LiteralPath $ini)) { throw 'Specify the active portable Rainmeter directory' }
$skinRoot=$null
foreach ($line in Get-Content -LiteralPath $ini) {
    if ($line -match '^SkinPath=(.+)$') { $skinRoot=[Environment]::ExpandEnvironmentVariables($Matches[1]); break }
}
if (!$skinRoot -or !(Test-Path -LiteralPath $skinRoot)) { throw 'Rainmeter SkinPath missing' }
$skin=Join-Path $skinRoot 'CLIProxyAccounts'
if ((Test-Path -LiteralPath $skin) -and !(Test-Path -LiteralPath (Join-Path $skin '.cliproxy-widget'))) { throw 'Refusing to overwrite an unrelated skin' }
Protect-WidgetDirectory $root
if (Test-Path -LiteralPath (Join-Path $root 'Helper.ps1')) {
    [IO.File]::WriteAllText((Join-Path $root 'stop'),'stop')
    $mutex=New-Object Threading.Mutex($false, ('Local\CLIProxyAccounts-'+[Security.Principal.WindowsIdentity]::GetCurrent().User.Value))
    try {
        if (!$mutex.WaitOne(10000)) { throw 'Existing helper did not stop; installation retained' }
        $mutex.ReleaseMutex()
    } finally { $mutex.Dispose() }
}
if ($ImportExistingOraclerKey) {
    # Capture stdout only in memory. Neither the key nor OAuth material is passed in arguments.
    $plain = & ssh -o BatchMode=yes oracler 'cat ~/.local/share/cliproxyapi/secrets/management-key.txt'
    if ($LASTEXITCODE -ne 0 -or !$plain) { throw 'Existing SSH key retrieval failed' }
    try { $ManagementKey=ConvertTo-SecureString ([string]$plain).Trim() -AsPlainText -Force } finally { $plain=$null }
}
if ($ManagementKey) { Save-WidgetKey $ManagementKey $root }
elseif (!(Test-Path -LiteralPath (Join-Path $root 'key.dpapi'))) { throw 'Supply ManagementKey as SecureString or import the authorized existing Oracler key' }
else {
    $existing=Get-WidgetKey $root
    try { Save-WidgetKey (ConvertTo-SecureString $existing -AsPlainText -Force) $root } finally { $existing=$null }
}
foreach ($name in @('Core.ps1','Helper.ps1','Start.ps1','Stop.ps1','OpenConsole.ps1','Uninstall.ps1')) { Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination $root -Force }
@{ BaseUrl=$base; RainmeterDirectory=$RainmeterDirectory; SkinDirectory=$skin } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $root 'settings.json') -Encoding UTF8
[void][IO.Directory]::CreateDirectory($skin)
Copy-Item -Path (Join-Path $PSScriptRoot 'CLIProxyAccounts\*') -Destination $skin -Recurse -Force
[IO.File]::WriteAllText((Join-Path $skin '.cliproxy-widget'), 'CLIProxyAccounts v1')
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'Start.ps1') -Destination (Join-Path $skin '@Resources\Start.ps1') -Force
# A fixed local command opens the validated configured console URL. No URL or API text enters Rainmeter actions.
$launcher='& (Join-Path $env:LOCALAPPDATA ''CLIProxyAccounts\OpenConsole.ps1'')'
[IO.File]::WriteAllText((Join-Path $skin '@Resources\Open.ps1'), $launcher)
$shell=New-Object -ComObject WScript.Shell
$shortcut=$shell.CreateShortcut((Join-Path $root 'Start CLIProxyAccounts.lnk'))
$shortcut.TargetPath="$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe"
$shortcut.Arguments='-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "'+(Join-Path $root 'Start.ps1')+'"'
$shortcut.WindowStyle=7; $shortcut.Save()
if ($StartWithWindows) { Copy-Item -LiteralPath (Join-Path $root 'Start CLIProxyAccounts.lnk') -Destination ([Environment]::GetFolderPath('Startup')) -Force }
& $exe '!RefreshApp'
& $exe '!ActivateConfig' 'CLIProxyAccounts' 'Accounts.ini'
& (Join-Path $root 'Start.ps1')
Write-Output "Installed helper: $root"
Write-Output "Installed skin: $skin"
