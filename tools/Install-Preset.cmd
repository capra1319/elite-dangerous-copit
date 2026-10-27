@echo off
setlocal
set "ELITEAUX_PRESET=%~dp0..\app\EliteAuxPanel.4.2.binds"
if exist "%~dp0EliteAuxPanel.4.2.binds" set "ELITEAUX_PRESET=%~dp0EliteAuxPanel.4.2.binds"
powershell -NoProfile -Command "$ErrorActionPreference='Stop'; $source=$env:ELITEAUX_PRESET; if (-not (Test-Path -LiteralPath $source)) { throw 'Packaged binding preset not found.' }; $folder=Join-Path $env:LOCALAPPDATA 'Frontier Developments\Elite Dangerous\Options\Bindings'; $target=Join-Path $folder 'EliteAuxPanel.4.2.binds'; if (-not (Test-Path -LiteralPath $folder)) { throw 'Elite Dangerous bindings folder not found. Run the game and save your controls first.' }; if (Test-Path -LiteralPath $target) { if ((Get-FileHash -LiteralPath $source).Hash -ne (Get-FileHash -LiteralPath $target).Hash) { throw 'EliteAuxPanel preset already exists and differs. No file was overwritten.' } } else { Copy-Item -LiteralPath $source -Destination $target }; Write-Host 'Preset installed. Restart Elite, then select EliteAuxPanel in Options - Controls - Ship Controls.'"
pause
