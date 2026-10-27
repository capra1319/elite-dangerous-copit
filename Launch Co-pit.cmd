@echo off
setlocal
title Co-pit

if exist "%~dp0build\bin\Co-pit.exe" goto launch

echo Co-pit has not been built in this folder yet.
echo.
where go.exe >nul 2>nul
if errorlevel 1 goto missinggo

echo The first build needs Go 1.26 or newer and internet access.
choice /C YN /N /M "Build Co-pit now? [Y/N] "
if errorlevel 2 exit /b 0
if errorlevel 1 goto build
exit /b 1

:build
powershell.exe -NoProfile -File "%~dp0Build-Co-pit.ps1"
if errorlevel 1 goto buildfailed
if not exist "%~dp0build\bin\Co-pit.exe" goto buildfailed

:launch
start "" /D "%~dp0build\bin" "%~dp0build\bin\Co-pit.exe"
if errorlevel 1 goto launchfailed
exit /b 0

:missinggo
echo Install Go 1.26 or newer to build this source copy.
echo Then open this launcher again.
echo See docs\development.md for build instructions.
echo.
pause
exit /b 1

:buildfailed
echo.
echo Co-pit could not be built. See the build error above.
echo See docs\development.md for help.
pause
exit /b 1

:launchfailed
echo.
echo Co-pit could not be opened from build\bin.
pause
exit /b 1
