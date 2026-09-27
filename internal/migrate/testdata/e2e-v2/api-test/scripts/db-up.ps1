# Oracle コンテナを起動し、LIBAPP スキーマを (再) 作成する。
# 何度実行しても同じ初期状態 (テーブル定義のみ・データなし) になる。
# データはテスト実行時に runnora の before フック (sql/common/*.sql) が投入する。
param(
    [int]$TimeoutSec = 600
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$container = 'runnora-e2e-oracle'
Push-Location $root
try {
    docker compose up -d oracle
    if ($LASTEXITCODE -ne 0) { throw 'docker compose up に失敗しました' }

    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    do {
        $health = docker inspect -f '{{.State.Health.Status}}' $container
        if ($health -eq 'healthy') { break }
        Write-Host "Oracle 起動待ち ($health) ..."
        Start-Sleep -Seconds 10
    } while ((Get-Date) -lt $deadline)
    if ($health -ne 'healthy') { throw "Oracle が $TimeoutSec 秒以内に healthy になりませんでした" }

    docker exec $container sqlplus -s -L '/ as sysdba' '@/opt/runnora-e2e/db/01_create_user.sql'
    if ($LASTEXITCODE -ne 0) { throw 'ユーザー作成に失敗しました' }
    docker exec $container sqlplus -s -L 'libapp/libapp_pw@//localhost:1521/FREEPDB1' '@/opt/runnora-e2e/db/02_schema.sql'
    if ($LASTEXITCODE -ne 0) { throw 'スキーマ作成に失敗しました' }
    Write-Host 'LIBAPP スキーマを作成しました (oracle://libapp:libapp_pw@127.0.0.1:1522/FREEPDB1)'
} finally {
    Pop-Location
}
