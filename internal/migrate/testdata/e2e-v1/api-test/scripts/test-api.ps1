# 実 API (Go + Oracle) に対してテストを流す。runbook ごとに PL/SQL の前後処理が走る。
#   1. generated : 生成 suite            (+ sql/cases/contract_setup.sql)
#   2. contract  : 契約テスト             (+ sql/cases/contract_setup.sql)
#   3. scenarios : シナリオ試験 LIB-001〜007 (scripts/scenarios.psd1 のケース固有 SQL 付き)
#   4. demo      : 事後検証が DB 不整合を検知して exit 4 になることの確認
# 最後に OpenAPI カバレッジを表示する。
# 事前に scripts/db-up.ps1 と scripts/api-start.ps1 (別ターミナル) を実行しておく。
param([string]$ReportDir)
. (Join-Path $PSScriptRoot '_common.ps1')
if (-not $ReportDir) { $ReportDir = New-ReportDir 'api' }
$defs = Import-PowerShellDataFile (Join-Path $PSScriptRoot 'scenarios.psd1')
Push-Location $Root
try {
    Wait-Http "$ApiUrl/health" 10
    $env:RUNNORA_BASE_URL = $ApiUrl
    Write-Host "== 実 API ($ApiUrl) に対するテスト"
    $results = @()
    $results += Invoke-Runnora -Name 'api-generated' -Config 'config.yaml' -ReportDir $ReportDir `
        -BeforeSql 'sql/cases/contract_setup.sql' `
        -Runbooks (Get-ChildItem runbooks/generated -Recurse -Filter *.suite.yml | Resolve-Path -Relative)
    $results += Invoke-Runnora -Name 'api-contract' -Config 'config.yaml' -ReportDir $ReportDir `
        -BeforeSql 'sql/cases/contract_setup.sql' `
        -Runbooks (Get-ChildItem runbooks/contract -Filter *.suite.yml | Resolve-Path -Relative)
    foreach ($s in $defs.Scenarios) {
        $results += Invoke-Runnora -Name $s.Id -Config 'config.yaml' -ReportDir $ReportDir `
            -BeforeSql $s.BeforeSql -AfterSql $s.AfterSql -Expect $s.Expect -Runbooks @($s.Runbook)
    }

    Write-Host '== OpenAPI カバレッジ (契約 + シナリオ)'
    $coverageTargets = @(Get-ChildItem runbooks/contract -Filter *.suite.yml | Resolve-Path -Relative) + @($defs.Scenarios | ForEach-Object { $_.Runbook })
    & $Runnora coverage --long @coverageTargets | Tee-Object -FilePath (Join-Path $ReportDir 'coverage.txt')

    $failed = @($results | Where-Object { -not $_.Ok })
    Write-Host ("実 API: {0}/{1} OK  レポート: {2}" -f ($results.Count - $failed.Count), $results.Count, $ReportDir)
    if ($failed.Count -gt 0) { exit 1 }
    exit 0
} finally {
    Remove-Item Env:RUNNORA_BASE_URL -ErrorAction SilentlyContinue
    Pop-Location
}
