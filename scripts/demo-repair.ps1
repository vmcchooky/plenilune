$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    New-Item -ItemType Directory -Path bin, out -Force | Out-Null
    go build -o bin/plenilune.exe ./cmd/plenilune
    if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
    $utf8 = [Text.UTF8Encoding]::new($false)
    $root = (Get-Location).Path
    $beforePath = Join-Path $root 'out/repair-before.acl'
    [IO.File]::WriteAllText($beforePath, "ip access-list extended MGMT`n 10 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22`n 20 deny ip any any`n", $utf8)
    $conditionLines = & ./bin/plenilune.exe obligations --file testdata/obligations.json
    if ($LASTEXITCODE -ne 0) { throw 'Condition normalization failed' }
    $conditions = ($conditionLines -join "`n") | ConvertFrom-Json
    $beforeHash = (Get-FileHash -LiteralPath $beforePath -Algorithm SHA256).Hash.ToLowerInvariant()
    $afterHash = (Get-FileHash -LiteralPath testdata/acl/management.acl -Algorithm SHA256).Hash.ToLowerInvariant()
    $policyPath = Join-Path $root 'out/repair-policy.json'
    $patchPath = Join-Path $root 'out/repair-patch.json'
    $proofPath = Join-Path $root 'out/repair-proof.json'
    $policy = @{schema='plenilune.policy/v1';scope=$conditions.conditions.scope;operator_accepted=$true;conditions=$conditions.conditions;allowed_changes=@()}
    $patch = @{schema='plenilune.patch/v1';scope=$conditions.conditions.scope;base_before_sha256=$beforeHash;base_after_sha256=$afterHash;obligations_digest=$conditions.sha256;edits=@(@{op='remove';ref=@{base_after_sha256=$afterHash;ordinal=0;line=3}})}
    [IO.File]::WriteAllText($policyPath, ($policy | ConvertTo-Json -Depth 20), $utf8)
    [IO.File]::WriteAllText($patchPath, ($patch | ConvertTo-Json -Depth 20), $utf8)
    $proof = & ./bin/plenilune.exe repair --before $beforePath --after testdata/acl/management.acl --policy $policyPath --patch $patchPath
    if ($LASTEXITCODE -ne 0) { throw 'Expected VERIFIED repair' }
    [IO.File]::WriteAllText($proofPath, (($proof -join "`n") + "`n"), $utf8)
    $result = ($proof -join "`n") | ConvertFrom-Json
    if ($result.status -ne 'VERIFIED' -or $result.collateral.status -ne 'VERIFIED') { throw 'Incomplete repair proof' }
    & ./bin/plenilune.exe repair-replay --before $beforePath --after testdata/acl/management.acl --evidence $proofPath
    if ($LASTEXITCODE -ne 0) { throw 'Repair replay failed' }
    $session = & ./bin/plenilune.exe assist --before $beforePath --after testdata/acl/management.acl --policy $policyPath --ai off
    if ($LASTEXITCODE -ne 0) { throw 'Offline session failed' }
    [IO.File]::WriteAllText((Join-Path $root 'out/offline-session.json'), (($session -join "`n") + "`n"), $utf8)
    Write-Output 'Synthetic VERIFIED repair + full collateral + rerender replay passed. Meta was not called.'
    Write-Output 'Proof: out/repair-proof.json'
} finally {
    Pop-Location
}
