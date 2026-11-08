$ErrorActionPreference = 'Stop'
$projectRoot = $PSScriptRoot
$admin = Join-Path $projectRoot 'bin/identity-admin.exe'
$sitegen = Join-Path $projectRoot 'bin/sitegen.exe'
if (!(Test-Path -LiteralPath $admin) -or !(Test-Path -LiteralPath $sitegen)) { & (Join-Path $projectRoot 'build.ps1') }
& $admin -catalog (Join-Path $projectRoot 'data/catalog.json') -referrals (Join-Path $projectRoot 'admin-panel/data/referrals.json') -builder $sitegen -site-out (Join-Path $projectRoot 'dist')
if ($LASTEXITCODE -ne 0) { throw 'Admin stopped with an error.' }
