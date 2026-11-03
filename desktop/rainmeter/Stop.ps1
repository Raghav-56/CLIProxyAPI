$root=Join-Path $env:LOCALAPPDATA 'CLIProxyAccounts'
if (Test-Path -LiteralPath $root) { [IO.File]::WriteAllText((Join-Path $root 'stop'), 'stop') }
