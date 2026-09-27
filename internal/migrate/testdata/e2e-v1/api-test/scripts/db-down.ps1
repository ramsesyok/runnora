# Oracle コンテナを停止・削除する (データはコンテナと一緒に消える)。
$ErrorActionPreference = 'Stop'
Push-Location (Split-Path $PSScriptRoot -Parent)
try {
    docker compose down
} finally {
    Pop-Location
}
