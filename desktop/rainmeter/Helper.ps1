param([switch]$Once)
. "$PSScriptRoot\Core.ps1"
$root=Get-WidgetRoot
$mutex=New-Object Threading.Mutex($false, ('Local\CLIProxyAccounts-'+[Security.Principal.WindowsIdentity]::GetCurrent().User.Value))
if (!$mutex.WaitOne(0)) { $mutex.Dispose(); exit 0 }
$lastConnected=''; $stop=Join-Path $root 'stop'
$stage='startup'
try {
    Remove-Item -LiteralPath $stop -Force -ErrorAction SilentlyContinue
    $stage='settings'
    $base=Get-PrivateRoot ((Get-Content -Raw -LiteralPath (Join-Path $root 'settings.json') | ConvertFrom-Json).BaseUrl)
    $stage='protected key'
    $key=Get-WidgetKey $root
    $stage='snapshot'
    do {
        $halt=$false
        try {
            $credentials=Invoke-ManagementRead $base 'credentials' $key $stop
            $route=Invoke-ManagementRead $base 'routing/codex-account' $key $stop
            $recent=Invoke-ManagementRead $base 'observability/requests/recent' $key $stop
            $now=[DateTimeOffset]::UtcNow
            $data=New-WidgetSnapshot $credentials $route $recent $now
            $lastConnected=$now.ToUnixTimeSeconds()
        } catch {
            $reason='Offline or invalid response'
            if ($_.Exception.Message -eq 'Authorization refused') { $reason='Authorization refused - stopped'; $halt=$true }
            $data=@{ Connection=$reason; Selected='unknown'; Primary='Primary unknown'; Secondary='Secondary unknown'; Pool='unknown'; Attempt='unknown' }
        }
        Write-WidgetSnapshot $data (Join-Path $root 'snapshot.ini') ([DateTimeOffset]::UtcNow) $lastConnected
        if ($Once -or $halt) { break }
        for ($i=0; $i -lt 150 -and !(Test-Path -LiteralPath $stop); $i++) { [Threading.Thread]::Sleep(100) }
    } while (!(Test-Path -LiteralPath $stop))
} catch {
    Write-WidgetSnapshot @{Connection=('Helper stopped at '+$stage)} (Join-Path $root 'snapshot.ini') ([DateTimeOffset]::UtcNow) ''
} finally {
    $key=$null
    if (Test-Path -LiteralPath $stop) {
        Write-WidgetSnapshot @{Connection='Helper stopped'; Selected='unknown'; Pool='unknown'} (Join-Path $root 'snapshot.ini') ([DateTimeOffset]::UtcNow) $lastConnected
    }
    $mutex.ReleaseMutex(); $mutex.Dispose()
}
