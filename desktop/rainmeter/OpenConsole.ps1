. "$PSScriptRoot\Core.ps1"
$root=Get-WidgetRoot
$base=Get-PrivateRoot ((Get-Content -Raw -LiteralPath (Join-Path $root 'settings.json') | ConvertFrom-Json).BaseUrl)
Start-Process ($base+'/accounts.html')
