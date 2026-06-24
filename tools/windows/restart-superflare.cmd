@echo off
setlocal
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0restart-superflare.ps1"
set "EXITCODE=%ERRORLEVEL%"
echo.
if not "%EXITCODE%"=="0" (
    echo restart-superflare failed with exit code %EXITCODE%.
)
pause
exit /b %EXITCODE%
