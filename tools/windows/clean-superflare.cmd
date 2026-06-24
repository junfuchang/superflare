@echo off
setlocal
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0clean-superflare.ps1"
set "EXITCODE=%ERRORLEVEL%"
echo.
if not "%EXITCODE%"=="0" (
    echo clean-superflare failed with exit code %EXITCODE%.
)
pause
exit /b %EXITCODE%
