$ErrorActionPreference = 'Stop'
$projectRoot = $PSScriptRoot
$env:GOCACHE = Join-Path $projectRoot '.cache/go-build'
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if ($goCommand) { $goExecutable = $goCommand.Source } else { $goExecutable = 'D:/Go/bin/go.exe' }
New-Item -ItemType Directory -Force -Path (Join-Path $projectRoot 'bin') | Out-Null
$sitegen = Join-Path $projectRoot 'bin/sitegen.exe'
$admin = Join-Path $projectRoot 'bin/identity-admin.exe'
& $goExecutable -C $projectRoot build -trimpath -o $sitegen ./cmd/sitegen
if ($LASTEXITCODE -ne 0) { throw 'Site generator build failed.' }
& $goExecutable -C $projectRoot build -trimpath -o (Join-Path $projectRoot 'bin/priceupdate.exe') ./cmd/priceupdate
if ($LASTEXITCODE -ne 0) { throw 'Price updater build failed.' }
& $goExecutable -C (Join-Path $projectRoot 'admin-panel') build -trimpath -o $admin .
if ($LASTEXITCODE -ne 0) { throw 'Admin build failed.' }
$generatorArguments = @('-catalog', (Join-Path $projectRoot 'data/catalog.json'), '-out', (Join-Path $projectRoot 'dist'))
$referrals = Join-Path $projectRoot 'admin-panel/data/referrals.json'
if (Test-Path -LiteralPath $referrals) { $generatorArguments += @('-referrals', $referrals) }
& $sitegen @generatorArguments
if ($LASTEXITCODE -ne 0) { throw 'Site generation failed; previous output was preserved.' }
Write-Host 'Build complete. Run ./start.ps1, then open http://127.0.0.1:8787/'
