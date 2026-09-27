# runnora-docgen で原稿を更新し、ddq で HTML を発行する。
param([switch]$GenerateOnly, [switch]$Pdf)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$projects = Split-Path (Split-Path $root -Parent) -Parent
$docgen = [Environment]::GetEnvironmentVariable('RUNNORA_DOCGEN_EXE')
if (-not $docgen) { $docgen = Join-Path $projects 'runnora-docgen\runnora-docgen.exe' }
if (-not (Test-Path -LiteralPath $docgen)) { throw "runnora-docgen が見つかりません: $docgen" }
Push-Location $root
try {
    & $docgen generate --base-dir . --config config.yaml --proto proto/library.proto `
        --out docs/generated --force runbooks/unary.yml runbooks/server-streaming.yml runbooks/calculation-streaming.yml runbooks/series-analysis.yml
    if ($LASTEXITCODE -ne 0) { throw '原稿生成に失敗しました' }
    if (-not $GenerateOnly) {
        ddq html docs
        if ($LASTEXITCODE -ne 0) { throw 'HTML 発行に失敗しました' }
        Write-Host (Join-Path $root 'docs\_book\index.html')
        if ($Pdf) {
            ddq pdf docs
            if ($LASTEXITCODE -ne 0) { throw 'PDF 発行に失敗しました' }
            Write-Host (Join-Path $root 'docs\design-doc.pdf')
        }
    }
} finally {
    Pop-Location
}
