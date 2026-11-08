$ErrorActionPreference = 'Stop'
$projectRoot = $PSScriptRoot
$updater = Join-Path $projectRoot 'bin/priceupdate.exe'
$config = Join-Path $projectRoot 'config/price-update.json'
if (!(Test-Path -LiteralPath $updater) -or !(Test-Path -LiteralPath $config)) { throw 'Build the updater and create the local config first.' }
$taskArguments = '-config "{0}" -catalog "{1}" -referrals "{2}" -out "{3}"' -f $config, (Join-Path $projectRoot 'data/catalog.json'), (Join-Path $projectRoot 'admin-panel/data/referrals.json'), (Join-Path $projectRoot 'dist')
$action = New-ScheduledTaskAction -Execute $updater -Argument $taskArguments -WorkingDirectory $projectRoot
$trigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddHours(24) -RepetitionInterval (New-TimeSpan -Hours 24)
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -MultipleInstances IgnoreNew -ExecutionTimeLimit (New-TimeSpan -Minutes 30)
$principal = New-ScheduledTaskPrincipal -UserId ([System.Security.Principal.WindowsIdentity]::GetCurrent().Name) -LogonType Interactive -RunLevel Limited
Register-ScheduledTask -TaskName 'SMSNav-DailyPrices' -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Description 'Read public SMS prices every 24 hours using the configured analysis API; no purchases.' -Force | Out-Null
Write-Host 'Daily task installed: SMSNav-DailyPrices. Run update-prices.cmd for the first update.'
