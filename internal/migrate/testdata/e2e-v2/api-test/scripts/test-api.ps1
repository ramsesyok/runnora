# 実 API (Go + Oracle) に対してテストを流す。runbook ごとに PL/SQL の前後処理が走る。
#   1. generated-unit : 生成 suite            (+ スイートの前処理 sql/cases/contract_setup.sql)
#   2. contract-unit  : 契約テスト             (+ スイートの前処理 sql/cases/contract_setup.sql)
#   3. scenarios      : シナリオ試験 LIB-001〜007 と検知デモ
#                       (ケース固有の SQL と期待する結果は各 runbook の runnora: ブロック。
#                        検知デモは expect: hookFail なので、事後検証が不整合を検知すれば合格)
# 最後に OpenAPI カバレッジを表示する。
# 接続先・共通の前後処理 (環境 unit) と実行する runbook は runnora.yaml のスイートに書いてある。
# 事前に scripts/db-up.ps1 と scripts/api-start.ps1 (別ターミナル) を実行しておく。
param([string]$ReportDir)
. (Join-Path $PSScriptRoot '_common.ps1')
if (-not $ReportDir) { $ReportDir = New-ReportDir 'api' }
Push-Location $Root
try {
    Wait-Http "$ApiUrl/health" 10
    Write-Host "== 実 API ($ApiUrl) に対するテスト"
    $results = @(
        Invoke-Runnora -Name 'api-generated' -Suite 'generated-unit' -ReportDir $ReportDir
        Invoke-Runnora -Name 'api-contract' -Suite 'contract-unit' -ReportDir $ReportDir
        Invoke-Runnora -Name 'api-scenarios' -Suite 'scenarios' -ReportDir $ReportDir
    )

    # loop で template を include する suite は runn の集計対象にならないため、契約テストは template を数える
    Write-Host '== OpenAPI カバレッジ (契約 + シナリオ)'
    & $Runnora coverage --long 'runbooks/contract/*.template.yml' 'runbooks/scenarios/*.yml' 'runbooks/demo/*.yml' |
        Tee-Object -FilePath (Join-Path $ReportDir 'coverage.txt')

    $failed = @($results | Where-Object { -not $_.Ok })
    Write-Host ("実 API: {0}/{1} OK  レポート: {2}" -f ($results.Count - $failed.Count), $results.Count, $ReportDir)
    if ($failed.Count -gt 0) { exit 1 }
    exit 0
} finally {
    Pop-Location
}
