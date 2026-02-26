@echo off
setlocal ENABLEDELAYEDEXPANSION

rem Ensure script runs from its own directory.
rem This also makes UNC paths work by mapping to a temporary drive letter.
pushd "%~dp0"

set APP_NAME=AuroraBorealisBliss
set OUT_DIR=%APP_NAME%-windows-app
set SCR_FILE=%APP_NAME%.scr
set LAUNCHER=%APP_NAME%.cmd

echo Building Windows screensaver binary...
go build -v -ldflags "-H windowsgui" -o "%SCR_FILE%" .
if errorlevel 1 (
  echo Build failed.
  popd
  exit /b 1
)

echo Preparing Windows app-like package...
if exist "%OUT_DIR%" rmdir /s /q "%OUT_DIR%"
mkdir "%OUT_DIR%"

copy /y "%SCR_FILE%" "%OUT_DIR%\\%SCR_FILE%" >nul

(
  echo @echo off
  echo start "" "%%~dp0%SCR_FILE%" /s
) > "%OUT_DIR%\\%LAUNCHER%"

echo Done: %OUT_DIR%
echo Run screensaver mode via:
echo   %OUT_DIR%\%LAUNCHER%
echo.
echo Or register %SCR_FILE% in Windows screensaver settings.

popd
endlocal

