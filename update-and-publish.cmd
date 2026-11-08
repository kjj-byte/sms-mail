@echo off
rem Daily job: capture prices, then publish dist/index.html to gh-pages.
setlocal
pushd "%~dp0"
bin\priceupdate.exe -config config\price-update.json -catalog data\catalog.json -referrals admin-panel\data\referrals.json -out dist
if errorlevel 1 (
  echo Price update failed; skipping publish. 1>&2
  popd
  exit /b 1
)
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0publish-dist.ps1"
set RC=%errorlevel%
popd
exit /b %RC%
