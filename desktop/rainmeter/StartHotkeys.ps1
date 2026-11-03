param(
    [switch]$Install,
    [switch]$Stop,
    [string]$Root = (Join-Path $env:LOCALAPPDATA 'CLIProxyAccounts')
)
$ErrorActionPreference='Stop'
$identity='Local\CLIProxyAccounts.Hotkeys.'+[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$lifecycle=New-Object Threading.Mutex($false, ($identity+'.Lifecycle'))
$locked=$false
$temporary=$null
try {
    try { $locked=$lifecycle.WaitOne(15000) } catch [Threading.AbandonedMutexException] { $locked=$true }
    if (!$locked) { throw 'Hotkey lifecycle operation is busy' }
    $exe=Join-Path $Root 'CLIProxyAccountsHotkeys.exe'
    if ($Install) {
        if (!(Test-Path -LiteralPath $Root -PathType Container)) { throw 'Protected install root must exist first' }
        $temporary=Join-Path $Root ([Guid]::NewGuid().ToString('N')+'.exe')
        $compiler=Join-Path $env:SystemRoot 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
        if (!(Test-Path -LiteralPath $compiler)) { $compiler=Join-Path $env:SystemRoot 'Microsoft.NET\Framework\v4.0.30319\csc.exe' }
        & $compiler /nologo /target:winexe /optimize+ ('/out:'+$temporary) (Join-Path $PSScriptRoot 'Hotkeys.cs')
        if ($LASTEXITCODE -ne 0) { throw 'Hotkey compilation failed; existing handler retained' }
    }
    if ($Stop -or $Install) {
        $owner=New-Object Threading.Mutex($false,$identity)
        try {
            $idle=$false
            try { $idle=$owner.WaitOne(0) } catch [Threading.AbandonedMutexException] { $idle=$true }
            if (!$idle) {
                $cancel=[Threading.EventWaitHandle]::OpenExisting($identity+'.Stop')
                try { [void]$cancel.Set() } finally { $cancel.Dispose() }
                try { $idle=$owner.WaitOne(10000) } catch [Threading.AbandonedMutexException] { $idle=$true }
                if (!$idle) { throw 'Hotkey handler did not stop; existing installation retained' }
            }
            if ($idle) { $owner.ReleaseMutex() }
        } finally { $owner.Dispose() }
        # The mutex is released just before process exit. Wait for the executable file lock too.
        foreach ($process in @(Get-Process -Name CLIProxyAccountsHotkeys -ErrorAction SilentlyContinue)) {
            try {
                if ($process.SessionId -eq (Get-Process -Id $PID).SessionId -and $process.Path -eq $exe) {
                    if (!$process.WaitForExit(5000)) { throw 'Hotkey handler has not exited' }
                }
            } finally { $process.Dispose() }
        }
    }
    if ($Stop) { return }
    if ($Install) {
        Move-Item -LiteralPath $temporary -Destination $exe -Force
        return
    }
    if (!(Test-Path -LiteralPath $exe)) { throw 'Install the native hotkey handler first' }
    $config=Get-Content -Raw -LiteralPath (Join-Path $Root 'settings.json') | ConvertFrom-Json
    $ready=New-Object Threading.EventWaitHandle($false,[Threading.EventResetMode]::ManualReset,($identity+'.Ready'))
    try {
        if ($ready.WaitOne(0)) { return }
        $process=Start-Process -FilePath $exe -ArgumentList @(('"'+(Join-Path $config.RainmeterDirectory 'Rainmeter.exe')+'"'),('"'+$config.SkinDirectory+'"')) -WindowStyle Hidden -PassThru
        try {
            if (!$ready.WaitOne(5000)) { throw 'Ctrl+Alt+Q registration failed; inspect hotkeys.log' }
        } finally { $process.Dispose() }
    } finally { $ready.Dispose() }
} finally {
    if ($temporary) { Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue }
    if ($locked) { $lifecycle.ReleaseMutex() }
    $lifecycle.Dispose()
}
