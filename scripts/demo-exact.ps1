$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    New-Item -ItemType Directory -Path bin, out -Force | Out-Null
    go build -o bin/plenilune.exe ./cmd/plenilune
    if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
    $utf8 = [Text.UTF8Encoding]::new($false)
    $before = "ip access-list extended MGMT`n 10 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22`n 20 deny ip any any`n"
    $beforePath = Join-Path (Get-Location) 'out/exact-before.acl'
    $artifactPath = Join-Path (Get-Location) 'out/comparison.json'
    [IO.File]::WriteAllText($beforePath, $before, $utf8)
    $json = & ./bin/plenilune.exe diff --before $beforePath --after testdata/acl/management.acl --obligations testdata/obligations.json
    if ($LASTEXITCODE -ne 1) { throw 'Expected DIFFERENT exit 1' }
    [IO.File]::WriteAllText($artifactPath, (($json -join "`n") + "`n"), $utf8)
    $artifact = ($json -join "`n") | ConvertFrom-Json
    if ($artifact.report.verdict -ne 'DIFFERENT' -or $artifact.report.obligations[0].status -ne 'VIOLATED') { throw 'Unexpected comparison or obligation result' }
    & ./bin/plenilune.exe replay --before $beforePath --after testdata/acl/management.acl --evidence $artifactPath
    if ($LASTEXITCODE -ne 0) { throw 'Full replay failed' }
    Write-Output 'Exact diff + obligation witness + full replay passed. Artifact: out/comparison.json'
} finally {
    Pop-Location
}
