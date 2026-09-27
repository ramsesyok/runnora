# WireMock (oapi2wire 生成モック) に対してテストを流す。DB は使わない。
#   1. generated : runnora generate が OpenAPI から作った suite (example 値・ステータスのみ検証)
#   2. contract  : 契約テスト (ステータス + 本文の一部 + OpenAPI 応答スキーマ検証)
# 事前に scripts/mock-build.ps1 と scripts/mock-start.ps1 (別ターミナル) を実行しておく。
param([string]$ReportDir)
. (Join-Path $PSScriptRoot '_common.ps1')
if (-not $ReportDir) { $ReportDir = New-ReportDir 'mock' }
Push-Location $Root
try {
    Wait-Http "$MockUrl/health" 10
    $env:RUNNORA_BASE_URL = $MockUrl
    Write-Host "== モック ($MockUrl) に対するテスト"
    $results = @(
        Invoke-Runnora -Name 'mock-generated' -Config 'config.mock.yaml' -ReportDir $ReportDir `
            -Runbooks (Get-ChildItem runbooks/generated -Recurse -Filter *.suite.yml | Resolve-Path -Relative)
        Invoke-Runnora -Name 'mock-contract' -Config 'config.mock.yaml' -ReportDir $ReportDir `
            -Runbooks (Get-ChildItem runbooks/contract -Filter *.suite.yml | Resolve-Path -Relative)
    )
    $failed = @($results | Where-Object { -not $_.Ok })
    Write-Host ("モック: {0}/{1} OK  レポート: {2}" -f ($results.Count - $failed.Count), $results.Count, $ReportDir)
    if ($failed.Count -gt 0) { exit 1 }
    exit 0
} finally {
    Remove-Item Env:RUNNORA_BASE_URL -ErrorAction SilentlyContinue
    Pop-Location
}
