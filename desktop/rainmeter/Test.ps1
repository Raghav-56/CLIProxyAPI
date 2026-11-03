. "$PSScriptRoot\Core.ps1"
$script:checks=0
function Assert($Condition,[string]$Label) {
    if (!$Condition) { throw "FAIL: $Label" }
    $script:checks++; Write-Output "PASS: $Label"
}
function Rejects([scriptblock]$Action,[string]$Label) {
    $rejected=$false
    try { & $Action | Out-Null } catch { $rejected=$true }
    Assert $rejected $Label
}
foreach ($file in Get-ChildItem $PSScriptRoot -Filter '*.ps1' -Recurse) {
    $tokens=$null; $errors=$null
    [void][Management.Automation.Language.Parser]::ParseFile($file.FullName,[ref]$tokens,[ref]$errors)
    Assert ($errors.Count -eq 0) ("PS runtime parses "+$file.Name)
}
Assert ($PSVersionTable.PSVersion.Major -ge 5) 'PowerShell 5 or newer'
foreach ($url in @('http://example.com','https://example.com','https://evil.ts.net.evil.com','http://100.64.1.1','https://user:pass@oracler.tail0f2859.ts.net','https://oracler.tail0f2859.ts.net/a','https://oracler.tail0f2859.ts.net/#x','http://127.0.0.1/?x','file:///C:/secret','http://127.1','http://2130706433')) {
    Rejects { Get-PrivateRoot $url } "Reject unsafe URL $url"
}
Assert ((Get-PrivateRoot 'https://oracler.tail0f2859.ts.net:8443') -eq 'https://oracler.tail0f2859.ts.net:8443') 'Private HTTPS accepted'
Assert ((Get-PrivateRoot 'http://127.0.0.1:8317') -eq 'http://127.0.0.1:8317') 'SSH loopback accepted'
$now=[DateTimeOffset]::Parse('2026-11-03T12:00:00Z')
function FixtureQuota {
    [pscustomobject]@{ observed_at=$now.AddSeconds(-60).ToString('o'); signals=[pscustomobject]@{
        'X-Codex-Primary-Used-Percent'='25'; 'X-Codex-Primary-Window-Minutes'='300'; 'X-Codex-Primary-Reset-After-Seconds'='600'
        'X-Codex-Secondary-Used-Percent'='10'; 'X-Codex-Secondary-Window-Minutes'='43200'; 'X-Codex-Secondary-Reset-At'=[string]$now.AddDays(20).ToUnixTimeSeconds()
    } }
}
$q=FixtureQuota; $view=Get-QuotaView $q $now
Assert ($view.State -eq 'available' -and $view.Primary -eq '5h : 75 pct left / reset 9m' -and $view.Secondary -like '30d : 90*') 'Observed durations and observation-relative reset'
$q.observed_at=$now.AddSeconds(-301).ToString('o')
Assert ((Get-QuotaView $q $now).State -eq 'stale') 'Older than five minutes stale'
$q.observed_at=$now.AddSeconds(1).ToString('o')
Assert ((Get-QuotaView $q $now).State -eq 'stale') 'Future observation rejected'
$q=FixtureQuota; $q.signals.'X-Codex-Primary-Reset-After-Seconds'='1'
Assert ((Get-QuotaView $q $now).State -eq 'reset-crossed') 'Reset crossed invalidates availability'
$q=FixtureQuota; $q.signals.'X-Codex-Primary-Used-Percent'='100'
Assert ((Get-QuotaView $q $now).State -eq 'exhausted') 'Exhaustion observed'
$q=FixtureQuota; $q.signals.'X-Codex-Primary-Used-Percent'='NaN'
Assert ((Get-QuotaView $q $now).State -eq 'unknown') 'Invalid numeric schema unknown'
Assert ((Get-QuotaView $null $now).State -eq 'unknown') 'Missing quota unknown'
$q=FixtureQuota; $q.observed_at=$now.AddSeconds(-300).ToString('o')
Assert ((Get-QuotaView $q $now).State -eq 'available') 'Five-minute boundary matches server policy'
$q=FixtureQuota; $q.signals | Add-Member -NotePropertyName 'X-Codex-Primary-Reset-At' -NotePropertyValue 'invalid'
Assert ((Get-QuotaView $q $now).State -eq 'unknown') 'Malformed reset-at does not fall back to reset-after'
$q=FixtureQuota; $q.signals | Add-Member -NotePropertyName 'X-Codex-Allowed' -NotePropertyValue 'false'
Assert ((Get-QuotaView $q $now).State -eq 'exhausted') 'Allowed false blocks quota'
$q=FixtureQuota; $q.signals | Add-Member -NotePropertyName 'X-Codex-Limit-Reached' -NotePropertyValue 'maybe'
Assert ((Get-QuotaView $q $now).State -eq 'unknown') 'Malformed flags remain unknown'
$q=FixtureQuota
Assert ((Get-QuotaView $q $now).ValidUntil -eq $now.AddSeconds(240).ToUnixTimeSeconds()) 'Snapshot freshness deadline does not renew observation'
$q=FixtureQuota
$credentials=[pscustomobject]@{files=@([pscustomobject]@{id='private@example.com.json';name='account.json';auth_index='index-a';account_type='oauth';runtime_only=$false;provider='codex';disabled=$false;unavailable=$false;quota=$q;id_token=[pscustomobject]@{plan_type='plus'}})}
$route=[pscustomobject]@{mode='manual';auth_id='private@example.com.json';applied=$true;strategy='';home_enabled=$false}
$recent=[pscustomobject]@{requests=@([pscustomobject]@{auth_id='private@example.com.json';model='gpt-5.1';success=$false;downstream_transport='sse';upstream_transport='websocket';latency_ms=1234.5;ttft_ms=$null;requested_at=$now.AddSeconds(-5).ToString('o')})}
$data=New-WidgetSnapshot $credentials $route $recent $now
Assert ($data.Selected -match '^acct-[0-9a-f]{8} / plus / available$' -and $data.Route -eq 'manual / applied') 'Manual selected and plan separate from pool'
Assert ($data.Transport -eq 'sse to websocket' -and $data.Timing -eq 'Elapsed 1234ms / TTFT unknown' -and $data.Attempt -like '*failed') 'Actual attempt transports timing and unknown TTFT'
Assert ($data.Tokens -eq 'Tokens unknown') 'Missing usage remains unknown'
$recent.requests[0] | Add-Member -NotePropertyName 'usage_known' -NotePropertyValue $true
$recent.requests[0] | Add-Member -NotePropertyName 'input_tokens' -NotePropertyValue 120
$recent.requests[0] | Add-Member -NotePropertyName 'output_tokens' -NotePropertyValue 0
$recent.requests[0] | Add-Member -NotePropertyName 'cached_input_tokens' -NotePropertyValue 80
Assert ((New-WidgetSnapshot $credentials $route $recent $now).Tokens -eq 'Tokens in/out/cache 120/0/80') 'Observed zero tokens distinct from unknown'
$recent.requests[0].cached_input_tokens=$null
Assert ((New-WidgetSnapshot $credentials $route $recent $now).Tokens -eq 'Tokens in/out/cache 120/0/unknown') 'Missing individual token count stays unknown'
$mixed=@($credentials.files[0])
foreach ($kind in @('api_key','runtime','missing-kind','missing-runtime','missing-identity')) {
    $entry=$credentials.files[0].PSObject.Copy()
    $entry.id=$kind
    if ($kind -eq 'api_key') { $entry.account_type='api_key' }
    if ($kind -eq 'runtime') { $entry.runtime_only=$true }
    if ($kind -eq 'missing-kind') { $entry.account_type=$null }
    if ($kind -eq 'missing-runtime') { $entry.runtime_only=$null }
    if ($kind -eq 'missing-identity') { $entry.auth_index='' }
    $mixed += $entry
}
$pool=New-WidgetSnapshot ([pscustomobject]@{files=$mixed}) $route $recent $now
Assert ($pool.Pool -eq '1 known available / 0 blocked / 0 unknown' -and $pool.PoolList -notmatch ',') 'Pool excludes API keys runtime and malformed credentials'
$route.mode='automatic'; $route.auth_id=''; $route.strategy='soonest-reset'
$auto=New-WidgetSnapshot $credentials $route $recent $now
Assert ($auto.Route -eq 'automatic soonest-reset / applied' -and $auto.Selected -eq 'No manual account') 'Automatic does not predict next account'
Rejects { New-WidgetSnapshot ([pscustomobject]@{}) $route $recent $now } 'Invalid top-level schema rejected'
Rejects { New-WidgetSnapshot ([pscustomobject]@{files='wrong'}) $route $recent $now } 'Invalid files array rejected'
$temp=Join-Path $env:TEMP ('cliproxy-widget-test-'+[Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($temp)
try {
    $path=Join-Path $temp 'snapshot.ini'
    $data.Selected="[!Execute evil]#Key#%ENV%`r`nOther=evil"
    Write-WidgetSnapshot $data $path $now $now.ToUnixTimeSeconds()
    $text=[IO.File]::ReadAllText($path)
    Assert ($text -notmatch 'private@example|!|#|%|Other=evil' -and $text -match 'Selected=Execute evil Key ENV Other evil') 'INI bang variable newline and identity sanitization'
    1..50 | ForEach-Object { Write-WidgetSnapshot $data $path $now $now.ToUnixTimeSeconds(); Assert ((Get-Content $path).Count -eq 18) 'Atomic replacement complete snapshot' }
    Assert (@(Get-ChildItem $temp -Filter '*.tmp').Count -eq 0) 'No atomic-write leftovers'
    Protect-WidgetDirectory $temp
    Save-WidgetKey (ConvertTo-SecureString 'fixture-secret-do-not-display' -AsPlainText -Force) $temp
    Assert ((Get-WidgetKey $temp) -eq 'fixture-secret-do-not-display') 'CurrentUser DPAPI round trip'
    Assert ((Get-Content -Raw (Join-Path $temp 'key.dpapi')) -notmatch 'fixture-secret') 'Secret absent from protected storage plaintext'
    Assert ((Get-Acl $temp).AreAccessRulesProtected) 'Credential directory ACL inheritance disabled'
    Assert ((Get-Acl (Join-Path $temp 'key.dpapi')).AreAccessRulesProtected) 'Key file has explicit restricted ACL'
    foreach ($code in @(401,403,302)) {
        $listener=New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback,0)
        $listener.Start(); $port=$listener.LocalEndpoint.Port
        $accept=$listener.AcceptTcpClientAsync()
        $job=Start-Job -ArgumentList $PSScriptRoot,$port,$temp -ScriptBlock {
            param($source,$port,$temp)
            . "$source\Core.ps1"
            try { Invoke-ManagementRead "http://127.0.0.1:$port" 'credentials' 'fixture-key' (Join-Path $temp 'stop'); 'unexpected success' }
            catch { $_.Exception.Message }
        }
        try {
            $client=$accept.GetAwaiter().GetResult(); $stream=$client.GetStream()
            $buffer=New-Object byte[] 4096; [void]$stream.Read($buffer,0,$buffer.Length)
            $wire=[Text.Encoding]::ASCII.GetBytes("HTTP/1.1 $code Refused`r`nLocation: http://127.0.0.1:1/evil`r`nContent-Length: 0`r`nConnection: close`r`n`r`n")
            $stream.Write($wire,0,$wire.Length); $client.Close()
            $job | Wait-Job | Out-Null; $result=$job | Receive-Job
            $expected='Management read failed'; if ($code -in @(401,403)) { $expected='Authorization refused' }
            Assert ($result -eq $expected) "HTTP $code stops without following redirect or exposing secret"
        } finally { $listener.Stop(); $job | Remove-Job -Force }
    }
    $listener=New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback,0)
    $listener.Start(); $port=$listener.LocalEndpoint.Port; $accept=$listener.AcceptTcpClientAsync()
    $job=Start-Job -ArgumentList $PSScriptRoot,$port,$temp -ScriptBlock {
        param($source,$port,$temp)
        . "$source\Core.ps1"
        try { Invoke-ManagementRead "http://127.0.0.1:$port" 'credentials' 'fixture-key' (Join-Path $temp 'stop'); 'unexpected success' }
        catch { $_.Exception.Message }
    }
    try {
        $client=$accept.GetAwaiter().GetResult()
        [IO.File]::WriteAllText((Join-Path $temp 'stop'),'stop')
        $finished=$job | Wait-Job -Timeout 10
        Assert ($null -ne $finished -and ($job | Receive-Job) -eq 'Stopped') 'Stop cancels an established stalled connection without network timeout'
        $client.Close()
    } finally { $listener.Stop(); $job | Remove-Job -Force }
} finally {
    # The target was created by this invocation under the OS temp directory.
    if ([IO.Path]::GetFullPath($temp).StartsWith([IO.Path]::GetFullPath($env:TEMP)+'\') -and (Split-Path $temp -Leaf) -like 'cliproxy-widget-test-*') { Remove-Item -LiteralPath $temp -Recurse -Force }
}
Write-Output "$script:checks checks passed on PowerShell $($PSVersionTable.PSVersion)"
