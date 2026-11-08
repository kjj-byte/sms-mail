@echo off
setlocal
if not exist "%~dp0bin\identity-admin.exe" (
  echo Build the executables first. See README.md for Go commands.
  exit /b 1
)
if not exist "%~dp0bin\sitegen.exe" (
  echo Build the executables first. See README.md for Go commands.
  exit /b 1
)

rem Auto-pick a free port starting from 8787 so a stale/other process cannot block startup.
set "ADDR=127.0.0.1:8787"
for /f "usebackq delims=" %%P in (`powershell -NoProfile -ExecutionPolicy Bypass -Command "$p=8787; while (Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue) { $p++ }; $p"`) do set "PORT=%%P"
if defined PORT set "ADDR=127.0.0.1:%PORT%"
echo [start] admin listening on %ADDR%

rem Delay the browser a bit so it does not race the server's listen; the exe below blocks in the foreground.
start "" /b powershell -NoProfile -ExecutionPolicy Bypass -Command "Start-Sleep -Milliseconds 1500; Start-Process 'http://%ADDR%/'"

"%~dp0bin\identity-admin.exe" -catalog "%~dp0data\catalog.json" -referrals "%~dp0admin-panel\data\referrals.json" -builder "%~dp0bin\sitegen.exe" -site-out "%~dp0dist" -addr %ADDR%
