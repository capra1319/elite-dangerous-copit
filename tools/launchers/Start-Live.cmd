@echo off
setlocal
set "ELITEAUX_BIN=%~dp0..\..\build\bin"
if not exist "%ELITEAUX_BIN%\eliteaux.exe" goto missing
pushd "%ELITEAUX_BIN%"
"%ELITEAUX_BIN%\eliteaux.exe" -mode live
set "ELITEAUX_EXIT=%errorlevel%"
popd
exit /b %ELITEAUX_EXIT%

:missing
echo Build the terminal app first with tools\Build-EliteAux.ps1.
echo See docs\development.md for instructions.
pause
exit /b 1
