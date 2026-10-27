param()
$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path $PSScriptRoot -Parent
$appRoot = Join-Path $repositoryRoot 'app'
$outputRoot = Join-Path $repositoryRoot 'build\bin'
Push-Location $repositoryRoot
try {
    New-Item -ItemType Directory -Path $outputRoot -Force | Out-Null
    go build -trimpath -o (Join-Path $outputRoot 'eliteaux.exe') ./app
    if ($LASTEXITCODE -ne 0) { throw 'Terminal build failed.' }
    Copy-Item -LiteralPath (Join-Path $repositoryRoot 'LICENSE'),(Join-Path $repositoryRoot 'COMMERCIAL.md') -Destination $outputRoot -Force
    Copy-Item -LiteralPath (Join-Path $appRoot 'control.json'),(Join-Path $appRoot 'EliteAuxPanel.4.2.binds'),(Join-Path $PSScriptRoot 'Install-Preset.cmd') -Destination $outputRoot -Force
    Copy-Item -LiteralPath (Join-Path $repositoryRoot 'licenses') -Destination $outputRoot -Recurse -Force
    Write-Host "Elite Aux is ready: $outputRoot\eliteaux.exe"
} finally { Pop-Location }
