@echo off
setlocal
"%~dp0bin\priceupdate.exe" -config "%~dp0config\price-update.json" -catalog "%~dp0data\catalog.json" -referrals "%~dp0admin-panel\data\referrals.json" -out "%~dp0dist" -watch
