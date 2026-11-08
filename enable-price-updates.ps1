$ErrorActionPreference = 'Stop'
$projectRoot = $PSScriptRoot
$updater = Join-Path $projectRoot 'bin/priceupdate.exe'
$config = Join-Path $projectRoot 'config/price-update.json'
if (!(Test-Path -LiteralPath $updater) -or !(Test-Path -LiteralPath $config)) { throw 'Build the updater and create the local config first.' }
# The daily task runs update-and-publish.cmd: capture prices, then push dist/index.html to gh-pages.
$taskRunner = Join-Path $projectRoot 'update-and-publish.cmd'
if (!(Test-Path -LiteralPath $taskRunner)) { throw 'update-and-publish.cmd is missing.' }
$action = New-ScheduledTaskAction -Execute $taskRunner -WorkingDirectory $projectRoot
$trigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddHours(24) -RepetitionInterval (New-TimeSpan -Hours 24)
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -MultipleInstances IgnoreNew -ExecutionTimeLimit (New-TimeSpan -Minutes 30)
$principal = New-ScheduledTaskPrincipal -UserId ([System.Security.Principal.WindowsIdentity]::GetCurrent().Name) -LogonType Interactive -RunLevel Limited
Register-ScheduledTask -TaskName 'SMSNav-DailyPrices' -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Description 'Capture public SMS prices daily and publish dist/index.html to the gh-pages branch; no purchases.' -Force | Out-Null
Write-Host 'Daily task installed: SMSNav-DailyPrices (updates prices then publishes to GitHub). Run update-prices.cmd for the first update.'
