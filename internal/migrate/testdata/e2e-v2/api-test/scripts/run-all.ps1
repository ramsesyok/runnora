# フルセットを一括実行する。
#   Oracle 起動・スキーマ作成 → モック生成 → テスト雛形生成 → API / WireMock をバックグラウンド起動
#   → モックのテスト → 実 API のテスト → 手順書生成 → API / WireMock 停止
# Oracle コンテナは残す (止めるときは scripts/db-down.ps1)。
param(
    [ValidateSet('Html', 'Pdf', 'Both', 'None')]
    [string]$DocFormat = 'Html',
    [switch]$SkipDbInit
)
. (Join-Path $PSScriptRoot '_common.ps1')
$jar = Resolve-Tool 'WIREMOCK_JAR' (Join-Path $Projects 'runnora\docs\tools\wiremock-standalone-3.13.2.jar') $null
$reportDir = New-ReportDir 'all'
$procs = @()
$failed = $false
Push-Location $Root
try {
    if (-not $SkipDbInit) { & (Join-Path $PSScriptRoot 'db-up.ps1') }
    & (Join-Path $PSScriptRoot 'mock-build.ps1')
    & (Join-Path $PSScriptRoot 'generate.ps1')

    go build -C api -o ../bin/library-api.exe .
    if ($LASTEXITCODE -ne 0) { throw 'API のビルドに失敗しました' }
    $procs += Start-Process -FilePath (Join-Path $Root 'bin\library-api.exe') -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $reportDir 'api-server.out.log') -RedirectStandardError (Join-Path $reportDir 'api-server.log')
    $procs += Start-Process -FilePath 'java' -PassThru -WindowStyle Hidden `
        -ArgumentList @('-jar', "`"$jar`"", '--port', '18080', '--bind-address', '127.0.0.1', '--root-dir', "`"$(Join-Path $Root 'mock\wiremock-out')`"", '--disable-banner') `
        -RedirectStandardOutput (Join-Path $reportDir 'wiremock.log') -RedirectStandardError (Join-Path $reportDir 'wiremock.err.log')
    Wait-Http "$ApiUrl/health" 60
    Wait-Http "$MockUrl/health" 60

    & (Join-Path $PSScriptRoot 'test-mock.ps1') -ReportDir $reportDir
    if ($LASTEXITCODE -ne 0) { $failed = $true }
    & (Join-Path $PSScriptRoot 'test-api.ps1') -ReportDir $reportDir
    if ($LASTEXITCODE -ne 0) { $failed = $true }
} finally {
    foreach ($p in $procs) {
        if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
    }
    Pop-Location
}
# ddq は PlantUML サーバを既定ポート 18080 で探すため、WireMock を止めてから手順書を作る
& (Join-Path $PSScriptRoot 'build-docs.ps1') -Format $DocFormat
if ($failed) {
    Write-Host "失敗したテストがあります。レポート: $reportDir" -ForegroundColor Red
    exit 1
}
Write-Host "すべて成功しました。レポート: $reportDir" -ForegroundColor Green
