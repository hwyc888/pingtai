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
echo The browser will open automatically. Configure the server on the web page.
echo After saving configuration, stop and start this launcher again to apply it.
echo Press Ctrl+C to stop the server.
echo.
pingtai.exe --open-browser
set "APP_EXIT=%ERRORLEVEL%"

if not "%APP_EXIT%"=="0" (
    echo.
    echo [ERROR] pingtai exited with code %APP_EXIT%.
    echo Check the listening port and folder permissions. Configuration is available at /setup.
    pause
)

exit /b %APP_EXIT%
