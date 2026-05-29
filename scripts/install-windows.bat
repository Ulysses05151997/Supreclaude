@echo off
REM ============================================================
REM  Mederos & Associates CRM - Windows installer
REM  RIGHT-CLICK this file and choose "Run as administrator".
REM ============================================================

setlocal
set PORT=8080

echo Installing the Mederos CRM service...
"%~dp0crm.exe" -config "%~dp0config.toml" install
if errorlevel 1 (
  echo.
  echo Install failed. Make sure you right-clicked and chose "Run as administrator".
  pause
  exit /b 1
)

echo Adding a Windows Firewall rule for port %PORT% ...
netsh advfirewall firewall add rule name="Mederos CRM" dir=in action=allow protocol=TCP localport=%PORT% >nul 2>&1

echo Starting the service...
"%~dp0crm.exe" -config "%~dp0config.toml" start

echo.
echo Done. Open this address on THIS computer to finish setup:
echo     http://localhost:%PORT%
echo.
echo To find this computer's address for the other office computers, open
echo Command Prompt and type:  ipconfig   (look for the IPv4 Address).
echo.
pause
