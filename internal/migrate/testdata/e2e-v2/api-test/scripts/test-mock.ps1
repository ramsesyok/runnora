# WireMock (oapi2wire 生成モック) に対してテストを流す。DB は使わない。
#   1. generated-mock : runnora generate が OpenAPI から作った suite (example 値・ステータスのみ検証)
#   2. contract-mock  : 契約テスト (ステータス + 本文の全体一致 + OpenAPI 応答スキーマ検証)
# 接続先 (環境 mock) と実行する runbook は runnora.yaml のスイートに書いてある。
# 事前に scripts/mock-build.ps1 と scripts/mock-start.ps1 (別ターミナル) を実行しておく。
param([string]$ReportDir)
. (Join-Path $PSScriptRoot '_common.ps1')
if (-not $ReportDir) { $ReportDir = New-ReportDir 'mock' }
Push-Location $Root
try {
    Wait-Http "$MockUrl/health" 10
    Write-Host "== モック ($MockUrl) に対するテスト"
    $results = @(
        Invoke-Runnora -Name 'mock-generated' -Suite 'generated-mock' -ReportDir $ReportDir
        Invoke-Runnora -Name 'mock-contract' -Suite 'contract-mock' -ReportDir $ReportDir
    )
    $failed = @($results | Where-Object { -not $_.Ok })
    Write-Host ("モック: {0}/{1} OK  レポート: {2}" -f ($results.Count - $failed.Count), $results.Count, $ReportDir)
    if ($failed.Count -gt 0) { exit 1 }
    exit 0
} finally {
    Pop-Location
}
