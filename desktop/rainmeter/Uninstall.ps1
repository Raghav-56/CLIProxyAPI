param([switch]$RemoveKey)
. "$PSScriptRoot\Core.ps1"
$root=Get-WidgetRoot
if (Test-Path -LiteralPath (Join-Path $root 'StartHotkeys.ps1')) { & (Join-Path $root 'StartHotkeys.ps1') -Stop }
$settings=Join-Path $root 'settings.json'
if (!(Test-Path -LiteralPath $settings)) { return }
$config=Get-Content -Raw -LiteralPath $settings | ConvertFrom-Json
& (Join-Path $root 'Stop.ps1')
$mutex=New-Object Threading.Mutex($false, ('Local\CLIProxyAccounts-'+[Security.Principal.WindowsIdentity]::GetCurrent().User.Value))
try {
    if (!$mutex.WaitOne(10000)) { throw 'Helper did not stop; installation retained' }
    try {
        $exe=Join-Path $config.RainmeterDirectory 'Rainmeter.exe'
        if (Test-Path -LiteralPath $exe) { & $exe '!DeactivateConfig' 'CLIProxyAccounts' }
        $skin=[IO.Path]::GetFullPath($config.SkinDirectory)
        if ([IO.Path]::GetFileName($skin.TrimEnd('\')) -ne 'CLIProxyAccounts' -or !(Test-Path -LiteralPath (Join-Path $skin '.cliproxy-widget'))) { throw 'Unrecognized skin; refusing removal' }
        Remove-Item -LiteralPath $skin -Recurse -Force
        $startup=Join-Path ([Environment]::GetFolderPath('Startup')) 'Start CLIProxyAccounts.lnk'
        if (Test-Path -LiteralPath $startup) {
            $shell=New-Object -ComObject WScript.Shell
            if ($shell.CreateShortcut($startup).Arguments -like ('*'+$root+'\Start.ps1*')) { Remove-Item -LiteralPath $startup -Force }
        }
        # Retain the protected directory and key unless explicitly requested. Remove only owned files.
        foreach ($name in @('Helper.ps1','Start.ps1','StartHotkeys.ps1','Hotkeys.cs','CLIProxyAccountsHotkeys.exe','hotkeys.log','Stop.ps1','OpenConsole.ps1','snapshot.ini','settings.json','Start CLIProxyAccounts.lnk','stop')) {
            Remove-Item -LiteralPath (Join-Path $root $name) -Force -ErrorAction SilentlyContinue
        }
        if ($RemoveKey) { Remove-Item -LiteralPath (Join-Path $root 'key.dpapi') -Force -ErrorAction SilentlyContinue }
        Write-Output 'CLIProxyAccounts removed. Other Rainmeter skins and layout entries retained.'
    } finally { $mutex.ReleaseMutex() }
} finally { $mutex.Dispose() }
