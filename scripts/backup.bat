@echo off
REM ============================================================
REM  Mederos & Associates CRM - database backup
REM  Safe to run while the service is running. Writes a
REM  timestamped snapshot next to the database.
REM  Tip: schedule this nightly with Windows Task Scheduler,
REM  and copy the backups off this machine (USB / cloud).
REM ============================================================

"%~dp0crm.exe" -config "%~dp0config.toml" backup
pause
