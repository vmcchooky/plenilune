param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path,
    [switch]$IncludeQA
)
$ErrorActionPreference = 'Stop'

$rootPath = (Resolve-Path -LiteralPath $Root).Path.TrimEnd('\')
$findings = New-Object System.Collections.Generic.List[string]

$excludePrefixes = @(
    'Sơ khai\',
    'out\',
    'bin\',
    'test-results\',
    'playwright-report\',
    'traces\',
    '.git\',
    'web\node_modules\',
    'web\dist\',
    'web\test-results\',
    'web\playwright-report\',
    'web\.vite\'
)
if (-not $IncludeQA) { $excludePrefixes += 'web\docs\qa\' }
$excludeFiles = @('web\tests\browser-qa.mjs')
$binaryExtensions = @(
    '.png', '.jpg', '.jpeg', '.gif', '.webp', '.ico', '.pdf', '.zip', '.gz',
    '.exe', '.dll', '.so', '.dylib', '.woff', '.woff2', '.ttf', '.otf',
    '.mp4', '.webm', '.lock'
)

$patterns = @(
    @{ Name = 'meta-env'; Regex = 'META_MODEL_API_KEY\s*=\s*[A-Za-z0-9_\-]{8,}' },
    @{ Name = 'openai-key'; Regex = '\bsk-[A-Za-z0-9]{20,}\b' },
    @{ Name = 'github-token'; Regex = '\b(gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{20,})\b' },
    @{ Name = 'aws-key'; Regex = '\bAKIA[0-9A-Z]{16}\b' },
    @{ Name = 'google-key'; Regex = '\bAIza[0-9A-Za-z_-]{35}\b' },
    @{ Name = 'slack-token'; Regex = '\bxox[baprs]-[A-Za-z0-9-]{10,}\b' },
    @{ Name = 'bearer-token'; Regex = '\bBearer\s+[A-Za-z0-9._~+/-]{20,}\b' },
    @{ Name = 'private-key'; Regex = '-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----' },
    @{ Name = 'generic-secret'; Regex = '(?i)\b(api[_-]?key|access[_-]?token|client[_-]?secret|password)\b\s*[:=]\s*["''][^"'']{12,}["'']' },
    @{ Name = 'local-user-path'; Regex = '[A-Za-z]:\\Users\\[^\\\s]+' }
)

if (Test-Path -LiteralPath (Join-Path $rootPath '.env')) {
    $findings.Add('.env [env-file-present]')
}

$scanned = 0
$files = Get-ChildItem -LiteralPath $rootPath -Recurse -File -Force -ErrorAction SilentlyContinue
foreach ($file in $files) {
    $relative = $file.FullName.Substring($rootPath.Length).TrimStart('\')
    $excluded = $false
    foreach ($prefix in $excludePrefixes) {
        if ($relative.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
            $excluded = $true
            break
        }
    }
    if ($excluded) { continue }
    if ($excludeFiles -contains $relative) { continue }
    if ($binaryExtensions -contains $file.Extension.ToLowerInvariant()) { continue }
    if ($file.Length -gt 8MB) {
        $findings.Add("$relative [file-too-large-to-scan]")
        continue
    }
    $scanned++
    $isTestPath = $relative -match '(^|\\)(internal|cmd)\\.*_test\.go$' -or $relative -match '^web\\tests\\'
    $lineNumber = 0
    foreach ($line in Get-Content -LiteralPath $file.FullName -ErrorAction SilentlyContinue) {
        $lineNumber++
        foreach ($pattern in $patterns) {
            if ($line -match $pattern.Regex) {
                # A generic assignment in a test path is allowed only when the
                # value itself is explicitly synthetic. High-confidence token
                # formats and all production paths are still blocked.
                if ($pattern.Name -eq 'generic-secret' -and $isTestPath -and
                    $line -match '(?i)(synthetic|dummy|example|test[-_ ]?key|not-a-real|placeholder|redacted)') {
                    break
                }
                $findings.Add("$relative`:$lineNumber [$($pattern.Name)]")
                break
            }
        }
    }
}

if ($findings.Count -gt 0) {
    $findings | ForEach-Object { Write-Output $_ }
    Write-Output "Secret scan found $($findings.Count) blocking item(s) in $scanned scanned files."
    exit 1
}
Write-Output "Secret scan clean: $scanned scanned files; no blocking patterns."
exit 0
