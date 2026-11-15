@echo off
setlocal
cd /d "%~dp0"

if not exist "pingtai.exe" (
    echo [ERROR] pingtai.exe was not found in:
    echo %CD%
    pause
    exit /b 1
)

echo Starting pingtai...
echo Working directory: %CD%
echo The browser will open automatically.
echo On first visit, create the first administrator account.
echo Manage AI providers and models from the platform admin page.
echo Press Ctrl+C to stop the server.
echo.
pingtai.exe --open-browser
set "APP_EXIT=%ERRORLEVEL%"

if not "%APP_EXIT%"=="0" (
    echo.
    echo [ERROR] pingtai exited with code %APP_EXIT%.
    echo Check the listening port, SQLite database path and folder permissions.
    pause
)

exit /b %APP_EXIT%
