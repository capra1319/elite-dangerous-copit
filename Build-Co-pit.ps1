param()
$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 build -tags desktop -skipbindings
    if ($LASTEXITCODE -ne 0) { throw 'Wails build failed.' }
    Copy-Item -LiteralPath control.json,EliteAuxPanel.4.2.binds -Destination build/bin -Force
    Copy-Item -LiteralPath licenses -Destination build/bin -Recurse -Force
    Write-Host "Co-pit is ready: $PSScriptRoot\build\bin\Co-pit.exe"
} finally { Pop-Location }
