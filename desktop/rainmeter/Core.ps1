# Windows PowerShell 5.1. No provider requests or credential refreshes.
$ErrorActionPreference = 'Stop'
# Start-Process can inherit another PowerShell edition's PSModulePath.
# Load the Windows-native credential cmdlets from this runtime explicitly.
Import-Module (Join-Path $PSHOME 'Modules\Microsoft.PowerShell.Security\Microsoft.PowerShell.Security.psd1') -ErrorAction Stop
function Get-WidgetRoot { Join-Path $env:LOCALAPPDATA 'CLIProxyAccounts' }
function Protect-WidgetDirectory([string]$Path) {
    [void][IO.Directory]::CreateDirectory($Path)
    $acl = New-Object Security.AccessControl.DirectorySecurity
    $acl.SetAccessRuleProtection($true, $false)
    foreach ($sid in @([Security.Principal.WindowsIdentity]::GetCurrent().User, (New-Object Security.Principal.SecurityIdentifier 'S-1-5-18'))) {
        $rule = New-Object Security.AccessControl.FileSystemAccessRule($sid, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')
        $acl.AddAccessRule($rule)
    }
    ([IO.DirectoryInfo]$Path).SetAccessControl($acl)
}
function Get-PrivateRoot([string]$Value) {
    if ($Value -notmatch '^https?://(?:127\.0\.0\.1|\[::1\]|[a-z0-9-]+\.[a-z0-9-]+\.ts\.net)(?::[0-9]{1,5})?/?$') { throw 'Invalid proxy root URL' }
    $uri = [Uri]$Value
    if (!$uri.IsAbsoluteUri -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or $uri.AbsolutePath -ne '/') { throw 'Invalid proxy root URL' }
    $ip = $null
    $literal = [Net.IPAddress]::TryParse($uri.DnsSafeHost, [ref]$ip)
    $loopback = $literal -and [Net.IPAddress]::IsLoopback($ip)
    # Remote names are restricted to authenticated Tailscale HTTPS names. No DNS rebinding to arbitrary HTTP.
    $tailnet = $uri.DnsSafeHost -match '^[a-z0-9-]+\.[a-z0-9-]+\.ts\.net$'
    if (!(($loopback -and $uri.Scheme -eq 'http') -or (($tailnet -or $loopback) -and $uri.Scheme -eq 'https'))) { throw 'Use private Tailscale HTTPS or an SSH loopback forward' }
    return $uri.GetLeftPart([UriPartial]::Authority)
}
function Save-WidgetKey([Security.SecureString]$Key, [string]$Root = (Get-WidgetRoot)) {
    Protect-WidgetDirectory $Root
    $path=Join-Path $Root 'key.dpapi'
    $Key | ConvertFrom-SecureString | Set-Content -LiteralPath $path -Encoding ASCII
    $acl=New-Object Security.AccessControl.FileSecurity
    $acl.SetAccessRuleProtection($true,$false)
    foreach ($sid in @([Security.Principal.WindowsIdentity]::GetCurrent().User, (New-Object Security.Principal.SecurityIdentifier 'S-1-5-18'))) {
        $acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($sid,'FullControl','Allow')))
    }
    ([IO.FileInfo]$path).SetAccessControl($acl)
}
function Get-WidgetKey([string]$Root) {
    $secure = (Get-Content -Raw -LiteralPath (Join-Path $Root 'key.dpapi')).Trim() | ConvertTo-SecureString
    $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try { [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr) } finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr) }
}
function Get-Field($Object, [string]$Name) {
    if ($null -ne $Object -and $null -ne $Object.PSObject.Properties[$Name]) { return $Object.$Name }
    return $null
}
function Get-Number($Value) {
    $n = 0.0
    if ($null -ne $Value -and [double]::TryParse([string]$Value, [Globalization.NumberStyles]::Float, [Globalization.CultureInfo]::InvariantCulture, [ref]$n) -and ![double]::IsNaN($n) -and ![double]::IsInfinity($n)) { return $n }
    return $null
}
function Get-Time($Value) {
    $time = [DateTimeOffset]::MinValue
    if ($Value -and [DateTimeOffset]::TryParse([string]$Value, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::AssumeUniversal, [ref]$time)) { return $time }
    return $null
}
function Get-MaskedID($Value) {
    if (!$Value) { return 'unknown' }
    $hash = [Security.Cryptography.SHA256]::Create()
    try { 'acct-' + ([BitConverter]::ToString($hash.ComputeHash([Text.Encoding]::UTF8.GetBytes([string]$Value)))).Replace('-','').Substring(0,8).ToLowerInvariant() } finally { $hash.Dispose() }
}
function Get-SafeDisplay($Value) {
    $text = [regex]::Replace([string]$Value, '[^a-zA-Z0-9 .,:_()/+\-]', ' ')
    return ([regex]::Replace($text, '\s+', ' ').Trim()).Substring(0, [Math]::Min(180, ([regex]::Replace($text, '\s+', ' ').Trim()).Length))
}
function Get-QuotaView($Quota, [DateTimeOffset]$Now) {
    $observed = Get-Time (Get-Field $Quota 'observed_at')
    $result = @{ State='unknown'; Primary='Primary unknown'; Secondary='Secondary unknown'; Age='unknown'; ValidUntil=0 }
    if (!$observed) { return $result }
    $age = ($Now - $observed).TotalSeconds
    $result.Age = [string][Math]::Max(0,[Math]::Floor($age)) + 's'
    if ($age -lt 0 -or $age -gt 300) { $result.State='stale'; return $result }
    $result.ValidUntil=$observed.AddSeconds(300).ToUnixTimeSeconds()
    $signals = Get-Field $Quota 'signals'
    $map = @{}
    if ($signals) {
        foreach ($p in $signals.PSObject.Properties) {
            $map[$p.Name] = [string]$p.Value
        }
    }
    $valid = 0; $malformed = $false; $crossed = $false
    $depleted = $map['X-Codex-Allowed'] -eq 'false' -or $map['X-Codex-Limit-Reached'] -eq 'true'
    foreach ($flag in @('X-Codex-Allowed','X-Codex-Limit-Reached')) {
        if ($map.ContainsKey($flag) -and $map[$flag] -notin @('true','false')) { $malformed=$true }
    }
    foreach ($name in @('Primary','Secondary')) {
        $prefix = 'X-Codex-' + $name + '-'
        if (!@($map.Keys | Where-Object { $_ -like "$prefix*" }).Count) { continue }
        $used = Get-Number $map[($prefix+'Used-Percent')]
        $minutes = Get-Number $map[($prefix+'Window-Minutes')]
        $reset = $null
        try {
            if ($map.ContainsKey($prefix+'Reset-At')) {
                $seconds = Get-Number $map[($prefix+'Reset-At')]
                if ($null -ne $seconds -and $seconds -gt 0) { $reset = [DateTimeOffset]::FromUnixTimeSeconds([long]$seconds) }
            } else {
                $seconds = Get-Number $map[($prefix+'Reset-After-Seconds')]
                if ($null -ne $seconds -and $seconds -ge 0) { $reset = $observed.AddSeconds($seconds) }
            }
        } catch { $reset=$null }
        if ($null -eq $used -or $used -lt 0 -or $used -gt 100 -or !$minutes -or $minutes -le 0 -or $minutes -ne [Math]::Floor($minutes) -or !$reset) { $malformed=$true; continue }
        $label = "$minutes min"
        if ($minutes % 1440 -eq 0) { $label = [string]($minutes/1440)+'d' }
        elseif ($minutes % 60 -eq 0) { $label = [string]($minutes/60)+'h' }
        if ($reset -le $Now) { $crossed=$true; $result[$name]="$label reset crossed"; continue }
        $result.ValidUntil=[Math]::Min($result.ValidUntil,$reset.ToUnixTimeSeconds())
        $left = [Math]::Round(100-$used,1).ToString([Globalization.CultureInfo]::InvariantCulture)
        $result[$name] = "$label : $left pct left / reset " + [Math]::Ceiling(($reset-$Now).TotalMinutes) + 'm'
        $valid++
        if ($used -eq 100) { $depleted=$true }
    }
    if ($crossed) { $result.State='reset-crossed' }
    elseif ($depleted) { $result.State='exhausted' }
    elseif (!$malformed -and $valid -gt 0) { $result.State='available' }
    return $result
}
function New-WidgetSnapshot($Credentials, $Route, $Recent, [DateTimeOffset]$Now) {
    if ($Credentials.files -isnot [Array] -or $Recent.requests -isnot [Array] -or (Get-Field $Route 'mode') -notin @('manual','automatic')) { throw 'Invalid management schema' }
    $rows = @($Credentials.files | Where-Object {
        (Get-Field $_ 'provider') -eq 'codex' -and (Get-Field $_ 'account_type') -eq 'oauth' -and
        (Get-Field $_ 'runtime_only') -eq $false -and
        (Get-Field $_ 'id') -is [string] -and ![string]::IsNullOrWhiteSpace($_.id) -and
        (Get-Field $_ 'name') -is [string] -and ![string]::IsNullOrWhiteSpace($_.name) -and
        (Get-Field $_ 'auth_index') -is [string] -and ![string]::IsNullOrWhiteSpace($_.auth_index)
    })
    $data = [ordered]@{ Connection='Connected'; Route=''; Selected='No manual account'; Primary='Primary unknown'; Secondary='Secondary unknown'; Observation='unknown'; Pool=''; PoolList=''; Attempt='No recent attempts'; Model='unknown'; Transport='unknown'; Timing='unknown'; Tokens='Tokens unknown'; AttemptAge='unknown'; QuotaUntil=$Now.AddSeconds(300).ToUnixTimeSeconds() }
    $mode = [string]$Route.mode
    if ($mode -ne 'manual') { $mode = 'automatic'; if ($Route.strategy -eq 'soonest-reset') { $mode='automatic soonest-reset' } }
    $applied = 'unconfirmed'; if ((Get-Field $Route 'applied') -eq $true) { $applied='applied' }
    $data.Route = "$mode / $applied"
    if ((Get-Field $Route 'home_enabled') -eq $true) { $data.Route='Home managed / '+$applied }
    $available=0; $unknown=0; $blocked=0; $list=@()
    foreach ($row in $rows) {
        $q = Get-QuotaView (Get-Field $row 'quota') $Now
        if ($q.ValidUntil -gt $Now.ToUnixTimeSeconds()) { $data.QuotaUntil=[Math]::Min($data.QuotaUntil,$q.ValidUntil) }
        $state=$q.State
        if ((Get-Field $row 'disabled') -isnot [bool] -or (Get-Field $row 'unavailable') -isnot [bool]) { $state='unknown' }
        if ((Get-Field $row 'disabled') -eq $true -or (Get-Field $row 'unavailable') -eq $true) { $state='unavailable' }
        if ($state -eq 'available') { $available++ } elseif ($state -in @('exhausted','unavailable')) { $blocked++ } else { $unknown++ }
        $id=Get-MaskedID (Get-Field $row 'id')
        $list += "$id $state"
        if ($Route.mode -eq 'manual' -and $row.id -eq $Route.auth_id) {
            $plan=Get-Field (Get-Field $row 'id_token') 'plan_type'
            if (!$plan) { $plan=Get-Field (Get-Field (Get-Field $row 'quota') 'signals') 'X-Codex-Plan-Type' }
            if ($plan -notin @('free','go','plus','pro','team','business','enterprise','edu')) { $plan='plan unknown' }
            $data.Selected="$id / $plan / $state"; $data.Primary=$q.Primary; $data.Secondary=$q.Secondary; $data.Observation=$q.Age
        }
    }
    if ($Route.mode -eq 'manual' -and $data.Selected -eq 'No manual account') { $data.Selected='Selected account missing' }
    $data.Pool="$available known available / $blocked blocked / $unknown unknown"
    $data.PoolList=($list | Select-Object -First 6) -join ', '
    if ($rows.Count -gt 6) { $data.PoolList += ', more in console' }
    $attempt = @($Recent.requests | Sort-Object { Get-Time (Get-Field $_ 'requested_at') } -Descending | Select-Object -First 1)
    if ($attempt.Count) {
        $r=$attempt[0]; $outcome='unknown'
        if ((Get-Field $r 'success') -eq $true) { $outcome='completed' } elseif ((Get-Field $r 'success') -eq $false) { $outcome='failed' }
        $data.Attempt='Latest '+(Get-MaskedID (Get-Field $r 'auth_id'))+' / '+$outcome
        $model=[string](Get-Field $r 'model')
        if ($model -match '^(gpt|o[134]|codex|claude|gemini)[a-zA-Z0-9._+\-]{0,60}$') { $data.Model=$model }
        $down=Get-Field $r 'downstream_transport'; $up=Get-Field $r 'upstream_transport'
        if ($down -notin @('http','sse','websocket')) { $down='unknown' }
        if ($up -notin @('http','sse','websocket')) { $up='unknown' }
        $data.Transport="$down to $up"
        $lat=Get-Number (Get-Field $r 'latency_ms'); $ttft=Get-Number (Get-Field $r 'ttft_ms')
        $l='unknown'; $t='unknown'
        if ($null -ne $lat -and $lat -ge 0) { $l=[string][Math]::Round($lat)+'ms' }
        if ($null -ne $ttft -and $ttft -ge 0) { $t=[string][Math]::Round($ttft)+'ms' }
        $data.Timing="Elapsed $l / TTFT $t"
        if ((Get-Field $r 'usage_known') -eq $true) {
            $counts=@()
            foreach ($field in @('input_tokens','output_tokens','cached_input_tokens')) {
                $count=Get-Number (Get-Field $r $field)
                if ($null -ne $count -and $count -ge 0 -and $count -eq [Math]::Floor($count)) {
                    $counts += $count.ToString('0',[Globalization.CultureInfo]::InvariantCulture)
                } else { $counts += 'unknown' }
            }
            $data.Tokens='Tokens in/out/cache '+($counts -join '/')
        }
        $at=Get-Time (Get-Field $r 'requested_at')
        if ($at -and $at -le $Now) { $data.AttemptAge=[string][Math]::Floor(($Now-$at).TotalSeconds)+'s' }
    }
    return $data
}
function Write-WidgetSnapshot($Data, [string]$Path, [DateTimeOffset]$Now, $ConnectedAt) {
    $lines=@('[Snapshot]', ('Written='+$Now.ToUnixTimeSeconds()), ('ConnectedAt='+$ConnectedAt), ('QuotaUntil='+(Get-SafeDisplay $Data['QuotaUntil'])))
    foreach ($name in @('Connection','Route','Selected','Primary','Secondary','Observation','Pool','PoolList','Attempt','Model','Transport','Timing','Tokens','AttemptAge')) { $lines += $name+'='+(Get-SafeDisplay $Data[$name]) }
    $temp=$Path+'.'+[Guid]::NewGuid().ToString('N')+'.tmp'
    try {
        [IO.File]::WriteAllLines($temp, $lines, [Text.Encoding]::ASCII)
        if ([IO.File]::Exists($Path)) { [IO.File]::Replace($temp,$Path,[System.Management.Automation.Language.NullString]::Value) } else { [IO.File]::Move($temp,$Path) }
    } finally { if ([IO.File]::Exists($temp)) { [IO.File]::Delete($temp) } }
}
function Invoke-ManagementRead([string]$Base, [string]$Endpoint, [string]$Key, [string]$StopFile) {
    if ($Endpoint -notin @('credentials','routing/codex-account','observability/requests/recent','server/info')) { throw 'Endpoint refused' }
    Add-Type -AssemblyName System.Net.Http
    $handler=New-Object Net.Http.HttpClientHandler
    $handler.AllowAutoRedirect=$false; $handler.UseProxy=$false
    $client=New-Object Net.Http.HttpClient($handler)
    $client.Timeout=[Threading.Timeout]::InfiniteTimeSpan
    $cancel=New-Object Threading.CancellationTokenSource
    try {
        $client.DefaultRequestHeaders.Authorization=New-Object Net.Http.Headers.AuthenticationHeaderValue('Bearer',$Key)
        $task=$client.GetAsync(($Base+'/v8/management/'+$Endpoint), $cancel.Token)
        while (!$task.IsCompleted) {
            if ([IO.File]::Exists($StopFile)) { $cancel.Cancel(); throw 'Stopped' }
            [Threading.Thread]::Sleep(100)
        }
        $response=$task.GetAwaiter().GetResult()
        try {
            $code=[int]$response.StatusCode
            if ($code -in @(401,403)) { throw 'Authorization refused' }
            if ($code -ne 200) { throw 'Management read failed' }
            $body=$response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
            if ($body.Length -gt 4194304) { throw 'Invalid management schema' }
            return ($body | ConvertFrom-Json)
        } finally { $response.Dispose() }
    } finally { $cancel.Dispose(); $client.Dispose(); $handler.Dispose() }
}
