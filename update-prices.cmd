@echo off
setlocal
if not exist "%~dp0bin\priceupdate.exe" (
  echo Build bin\priceupdate.exe first. See docs\price-updates.md.
  exit /b 1
)
"%~dp0bin\priceupdate.exe" -config "%~dp0config\price-update.json" -catalog "%~dp0data\catalog.json" -referrals "%~dp0admin-panel\data\referrals.json" -out "%~dp0dist"
