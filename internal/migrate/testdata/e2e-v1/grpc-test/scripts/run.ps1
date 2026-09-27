# Go 製 gRPC サーバを起動して Unary / Server streaming の runbook を実行する。
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$projects = Split-Path (Split-Path $root -Parent) -Parent
$runnora = [Environment]::GetEnvironmentVariable('RUNNORA_EXE')
$runnoraProject = Join-Path $projects 'runnora'
if ($runnora -and -not (Test-Path -LiteralPath $runnora)) { throw "runnora が見つかりません: $runnora" }
if (-not $runnora -and -not (Test-Path -LiteralPath (Join-Path $runnoraProject 'go.mod'))) {
    throw "runnora のソースが見つかりません: $runnoraProject"
}
$jsondiffProject = [Environment]::GetEnvironmentVariable('RUNNORA_DIFF_SOURCE_DIR')
if (-not $jsondiffProject) { $jsondiffProject = Join-Path $projects 'runnora-diff' }
if (-not (Test-Path -LiteralPath (Join-Path $jsondiffProject 'go.mod'))) {
    throw "runnora-diff のソースが見つかりません: $jsondiffProject"
}

Push-Location $root
$proc = $null
$previousEvidenceDir = [Environment]::GetEnvironmentVariable('RUNNORA_EVIDENCE_DIR')
try {
    New-Item -ItemType Directory -Force bin | Out-Null
    $reportDir = Join-Path $root ('reports\' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
    New-Item -ItemType Directory -Force $reportDir | Out-Null
    $env:RUNNORA_EVIDENCE_DIR = Join-Path $reportDir 'evidence'
    New-Item -ItemType Directory -Force $env:RUNNORA_EVIDENCE_DIR | Out-Null
    go build -buildvcs=false -o bin/libraryd.exe ./cmd/libraryd
    if ($LASTEXITCODE -ne 0) { throw 'gRPC サーバのビルドに失敗しました' }
    go -C $jsondiffProject build -buildvcs=false -o (Join-Path $root 'bin/runnora-diff.exe') .
    if ($LASTEXITCODE -ne 0) { throw 'runnora-diff のビルドに失敗しました' }
    if (-not $runnora) {
        $runnora = Join-Path $root 'bin/runnora.exe'
        go -C $runnoraProject build -buildvcs=false -o $runnora .
        if ($LASTEXITCODE -ne 0) { throw 'runnora のビルドに失敗しました' }
    }
    $proc = Start-Process -FilePath (Join-Path $root 'bin\libraryd.exe') -PassThru -WindowStyle Hidden `
        -ArgumentList @('-proto', 'proto/library.proto') `
        -RedirectStandardOutput (Join-Path $reportDir 'server.out.log') `
        -RedirectStandardError (Join-Path $reportDir 'server.err.log')
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        if ($proc.HasExited) { throw 'gRPC サーバが起動直後に終了しました（ポート重複または proto の読み込みエラー）' }
        $client = New-Object Net.Sockets.TcpClient
        try {
            $client.Connect('127.0.0.1', 19090)
            $ready = $true
            break
        } catch {
            Start-Sleep -Milliseconds 200
        } finally {
            $client.Dispose()
        }
    }
    if (-not $ready) { throw 'gRPC サーバが 127.0.0.1:19090 で起動しませんでした' }
    & $runnora run --config config.yaml --scopes run:exec --report-format text `
        --report-out (Join-Path $reportDir 'runnora.txt') `
        runbooks/unary.yml runbooks/server-streaming.yml runbooks/calculation-streaming.yml runbooks/series-analysis.yml
    if ($LASTEXITCODE -ne 0) {
        if (Test-Path -LiteralPath (Join-Path $reportDir 'runnora.txt')) {
            Get-Content (Join-Path $reportDir 'runnora.txt')
        }
        throw 'runbook の実行に失敗しました'
    }
    Get-Content (Join-Path $reportDir 'runnora.txt')
    & $runnora coverage --long runbooks/unary.yml runbooks/server-streaming.yml runbooks/calculation-streaming.yml runbooks/series-analysis.yml |
        Tee-Object -FilePath (Join-Path $reportDir 'coverage.txt')
    if ($LASTEXITCODE -ne 0) { throw 'カバレッジの確認に失敗しました' }
    Write-Host "レポート: $reportDir"
} finally {
    [Environment]::SetEnvironmentVariable('RUNNORA_EVIDENCE_DIR', $previousEvidenceDir)
    if ($proc -and -not $proc.HasExited) { Stop-Process -Id $proc.Id -Force }
    Pop-Location
}
