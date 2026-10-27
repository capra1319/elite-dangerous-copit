param()
$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path $PSScriptRoot -Parent
$appRoot = Join-Path $repositoryRoot 'app'
$outputRoot = Join-Path $repositoryRoot 'build\bin'
Push-Location $appRoot
try {
    go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 build -tags desktop -skipbindings
    if ($LASTEXITCODE -ne 0) { throw 'Wails build failed.' }
    Copy-Item -LiteralPath (Join-Path $repositoryRoot 'LICENSE'),(Join-Path $repositoryRoot 'COMMERCIAL.md') -Destination $outputRoot -Force
    Copy-Item -LiteralPath (Join-Path $appRoot 'control.json'),(Join-Path $appRoot 'EliteAuxPanel.4.2.binds'),(Join-Path $PSScriptRoot 'Install-Preset.cmd') -Destination $outputRoot -Force
    Copy-Item -LiteralPath (Join-Path $repositoryRoot 'licenses') -Destination $outputRoot -Recurse -Force
    Write-Host "Co-pit is ready: $outputRoot\Co-pit.exe"
} finally { Pop-Location }
