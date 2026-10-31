$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    New-Item -ItemType Directory -Path bin, out -Force | Out-Null
    go build -o bin/plenilune.exe ./cmd/plenilune
    if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
    & ./bin/plenilune.exe version
    if ($LASTEXITCODE -ne 0) { throw 'Version failed' }
    $trace = & ./bin/plenilune.exe trace --config testdata/acl/management.acl --packet testdata/packet.json
    if ($LASTEXITCODE -ne 0) { throw 'Trace failed' }
    $utf8 = [Text.UTF8Encoding]::new($false)
    [IO.File]::WriteAllText((Join-Path (Get-Location) 'out/trace.json'), (($trace -join "`n") + "`n"), $utf8)
    & ./bin/plenilune.exe verify --config testdata/acl/management.acl --evidence out/trace.json
    if ($LASTEXITCODE -ne 0) { throw 'Replay failed' }
    & ./bin/plenilune.exe obligations --file testdata/obligations.json
    if ($LASTEXITCODE -ne 0) { throw 'Obligation validation failed' }
    Write-Output 'Foundation demo passed. Trace: out/trace.json'
} finally {
    Pop-Location
}
