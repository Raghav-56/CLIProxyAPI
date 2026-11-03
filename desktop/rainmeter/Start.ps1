$root=Join-Path $env:LOCALAPPDATA 'CLIProxyAccounts'
try { & (Join-Path $root 'StartHotkeys.ps1') } catch { Write-Warning 'Ctrl+Alt+Q unavailable; inspect hotkeys.log' }
Start-Process -FilePath "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -ArgumentList @('-NoProfile','-NonInteractive','-WindowStyle','Hidden','-ExecutionPolicy','Bypass','-File', ('"'+(Join-Path $root 'Helper.ps1')+'"')) -WindowStyle Hidden
