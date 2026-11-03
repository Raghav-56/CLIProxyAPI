. "$PSScriptRoot\Core.ps1"
$root=Get-WidgetRoot
$base=Get-PrivateRoot ((Get-Content -Raw (Join-Path $root 'settings.json') | ConvertFrom-Json).BaseUrl)
$key=Get-WidgetKey $root
try {
    foreach ($endpoint in @('credentials','routing/codex-account','observability/requests/recent','server/info')) {
        $value=Invoke-ManagementRead $base $endpoint $key (Join-Path $root 'check-stop')
        Write-Output ($endpoint+' 200 fields: '+(($value.PSObject.Properties.Name | Sort-Object) -join ', '))
        if ($endpoint -eq 'credentials') {
            Write-Output ('credential count: '+@($value.files).Count)
            $quota=@($value.files | Where-Object { $_.provider -eq 'codex' -and $_.quota.signals } | Select-Object -First 1)
            if ($quota.Count) {
                Write-Output ('quota fields: '+($quota[0].quota.PSObject.Properties.Name -join ', '))
                Write-Output ('quota signal names: '+($quota[0].quota.signals.PSObject.Properties.Name -join ', '))
            }
        }
        if ($endpoint -eq 'routing/codex-account') { Write-Output ('route: '+$value.mode+' applied='+$value.applied+' selected-present='+[bool]$value.auth_id) }
        if ($endpoint -eq 'observability/requests/recent' -and @($value.requests).Count) { Write-Output ('request fields: '+($value.requests[0].PSObject.Properties.Name -join ', ')) }
    }
} catch {
    # No exception bodies, URLs, request headers, or upstream data in diagnostics.
    Write-Output 'Live read failed; stopped without retry.'
    exit 1
} finally { $key=$null }
