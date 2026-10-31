param(
    [string]$Destination = (Join-Path (Split-Path $PSScriptRoot -Parent) ('out/public-review-' + (Get-Date -Format 'yyyyMMdd-HHmmss')))
)
$ErrorActionPreference = 'Stop'

$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path.TrimEnd('\')
$destinationPath = [System.IO.Path]::GetFullPath($Destination)
if (Test-Path -LiteralPath $destinationPath) {
    throw "Destination already exists: $destinationPath. Choose a new folder."
}
if (-not $destinationPath.StartsWith((Join-Path $root 'out') + '\', [System.StringComparison]::OrdinalIgnoreCase)) {
    throw 'Public snapshots must be written under the ignored out/ folder.'
}
New-Item -ItemType Directory -Path $destinationPath -Force | Out-Null

$excludedRelative = @(
    'docs/public-manifest.sha256',
    'docs/handoff/PROMPT_DEMO_PUBLIC.vi.md',
    'docs/handoff/PROMPT_QA_PREFIX.vi.md',
    'docs/handoff/QA_NEXT.vi.md'
)

function Copy-PublicFile {
    param([System.IO.FileInfo]$File)
    $relative = $File.FullName.Substring($root.Length).TrimStart('\').Replace('\', '/')
    if ($excludedRelative -contains $relative) { return }
    if ($File.Name -eq '.DS_Store') { return }
    $target = Join-Path $destinationPath $relative
    $parent = Split-Path $target -Parent
    if (-not (Test-Path -LiteralPath $parent)) { New-Item -ItemType Directory -Path $parent -Force | Out-Null }
    Copy-Item -LiteralPath $File.FullName -Destination $target
}

$rootFiles = @(
    'README.md', 'README.vi.md', 'LICENSE', 'NOTICE', 'SECURITY.md',
    'THIRD_PARTY_NOTICES.md', 'AGENTS.md', 'ARCHITECTURE.md', 'CONTRACTS.md',
    'VERIFICATION_DESIGN.md', 'AI_AGENT_DESIGN.md', 'ENGINEERING_GUARDRAILS.md',
    'ROADMAP.md', 'REVIEW_NOTES.md', '.env.example', '.gitignore', 'go.mod'
)
foreach ($name in $rootFiles) { Copy-PublicFile (Get-Item -LiteralPath (Join-Path $root $name)) }

foreach ($tree in @('cmd', 'internal', 'testdata', 'docs', 'examples/moonstorm-client', 'web/src', 'web/public', 'third_party/licenses')) {
    $base = Join-Path $root $tree
    if (-not (Test-Path -LiteralPath $base)) { continue }
    Get-ChildItem -LiteralPath $base -Recurse -File -Force | ForEach-Object { Copy-PublicFile $_ }
}

Get-ChildItem -LiteralPath (Join-Path $root 'scripts') -File -Filter '*.ps1' | ForEach-Object { Copy-PublicFile $_ }

foreach ($name in @('index.html', 'package.json', 'package-lock.json', 'tsconfig.json', 'vite.config.ts', 'vitest.config.ts')) {
    Copy-PublicFile (Get-Item -LiteralPath (Join-Path $root ('web/' + $name)))
}

Get-ChildItem -LiteralPath (Join-Path $root 'web/tests') -File -Force |
    Where-Object { $_.Name -notin @('browser-qa.mjs', 'qa-ui-2026-10-31.mjs') } |
    ForEach-Object { Copy-PublicFile $_ }

$manifestPath = Join-Path $root 'docs/public-manifest.sha256'
$files = Get-ChildItem -LiteralPath $destinationPath -Recurse -File -Force | Sort-Object FullName
$lines = foreach ($file in $files) {
    $relative = $file.FullName.Substring($destinationPath.Length).TrimStart('\').Replace('\', '/')
    $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash *$relative"
}
$lines | Set-Content -LiteralPath $manifestPath -Encoding ascii
Copy-Item -LiteralPath $manifestPath -Destination (Join-Path $destinationPath 'docs/public-manifest.sha256')

Write-Output "Public snapshot: $destinationPath"
Write-Output "Manifest entries: $($lines.Count) (manifest file itself is not listed)"
Write-Output 'Run the secret scan against the snapshot before sharing:'
Write-Output "  powershell -NoProfile -ExecutionPolicy Bypass -File scripts/secret-scan.ps1 -Root '$destinationPath'"
