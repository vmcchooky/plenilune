param(
    [ValidateRange(1024, 65535)][int]$Port = 8082
)
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
$Host.UI.RawUI.WindowTitle = 'Plenilune - local Meta setup (Spark 1.3 Contributor only)'

Write-Host 'Plenilune local environment' -ForegroundColor Cyan
Write-Host 'Model: muse-spark-1.3-contributor (fixed for this session; no model fallback).'
Write-Host 'Startup does not call Meta. API key stays in process memory only.'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()
$form = New-Object System.Windows.Forms.Form
$form.Text = 'Plenilune - Meta API setup'
$form.ClientSize = New-Object System.Drawing.Size(540, 220)
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.TopMost = $true
$label = New-Object System.Windows.Forms.Label
$label.Text = "Model: muse-spark-1.3-contributor (fixed; no fallback).`r`nPaste your key below. No API call is made during startup."
$label.Location = New-Object System.Drawing.Point(18, 18)
$label.Size = New-Object System.Drawing.Size(500, 44)
$keyBox = New-Object System.Windows.Forms.TextBox
$keyBox.Location = New-Object System.Drawing.Point(18, 76)
$keyBox.Size = New-Object System.Drawing.Size(400, 26)
$keyBox.UseSystemPasswordChar = $true
$keyBox.ShortcutsEnabled = $true
$paste = New-Object System.Windows.Forms.Button
$paste.Text = 'Paste'
$paste.Location = New-Object System.Drawing.Point(430, 73)
$paste.Size = New-Object System.Drawing.Size(90, 30)
$paste.Add_Click({
    if ([System.Windows.Forms.Clipboard]::ContainsText()) {
        $keyBox.Text = [System.Windows.Forms.Clipboard]::GetText().Trim()
        $keyBox.Focus()
    }
})
$note = New-Object System.Windows.Forms.Label
$note.Text = 'Key stays in this running session only. Leave empty for offline mode.'
$note.Location = New-Object System.Drawing.Point(18, 116)
$note.Size = New-Object System.Drawing.Size(500, 24)
$start = New-Object System.Windows.Forms.Button
$start.Text = 'Start locally'
$start.Location = New-Object System.Drawing.Point(310, 165)
$start.Size = New-Object System.Drawing.Size(110, 32)
$start.DialogResult = [System.Windows.Forms.DialogResult]::OK
$cancel = New-Object System.Windows.Forms.Button
$cancel.Text = 'Cancel'
$cancel.Location = New-Object System.Drawing.Point(430, 165)
$cancel.Size = New-Object System.Drawing.Size(90, 32)
$cancel.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$form.Controls.AddRange(@($label, $keyBox, $paste, $note, $start, $cancel))
$form.AcceptButton = $start
$form.CancelButton = $cancel
$form.Add_Shown({ $keyBox.Focus() })
try {
    if ($form.ShowDialog() -ne [System.Windows.Forms.DialogResult]::OK) {
        exit
    }
    $env:META_MODEL_API_KEY = $keyBox.Text.Trim()
} finally {
    $keyBox.Clear()
    $form.Dispose()
}
$env:PLENILUNE_META_MODEL = 'muse-spark-1.3-contributor'
$env:PLENILUNE_META_BASE_URL = 'https://api.meta.ai/v1'

$demoBinary = Join-Path (Split-Path $PSScriptRoot -Parent) 'bin/plenilune-demo.exe'
if (-not (Test-Path -LiteralPath $demoBinary)) {
    throw 'Demo server not built. Run: go build -o bin/plenilune-demo.exe ./cmd/plenilune-demo'
}

try {
    if ($env:META_MODEL_API_KEY) {
        Write-Host 'Key loaded locally. No API request has been sent.' -ForegroundColor Green
    } else {
        Write-Host 'Offline mode: no key loaded.' -ForegroundColor Yellow
    }
    $localUrl = "http://127.0.0.1:$Port"
    Write-Host "Open $localUrl - close this window to stop the server."
    Write-Host 'The interface uses the local engine. Meta runs only after explicit opt-in.'
    $bravePaths = @(
        'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe',
        'C:/Program Files (x86)/BraveSoftware/Brave-Browser/Application/brave.exe',
        "$env:LOCALAPPDATA/BraveSoftware/Brave-Browser/Application/brave.exe"
    )
    $brave = $bravePaths | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    if ($brave) {
        Start-Process -FilePath $brave -ArgumentList $localUrl
    }
    & $demoBinary -addr "127.0.0.1:$Port" -web 'web/dist'
} finally {
    Remove-Item Env:META_MODEL_API_KEY -ErrorAction SilentlyContinue
}
