# OpenAPI からテストの雛形 (runbooks/generated/, cases/generated/) を作り直す。
# 生成後に応答証跡と独立した判定を追加する。育てるテストは runbooks/contract/ に複製して使う。
. (Join-Path $PSScriptRoot '_common.ps1')
Push-Location $Root
try {
    & $Runnora generate --config config.mock.yaml --openapi openapi/library-api.yaml --out . `
        --clean --force
    if ($LASTEXITCODE -ne 0) { throw 'runnora generate に失敗しました' }
    & (Join-Path $PSScriptRoot 'add-response-evidence.ps1')
} finally {
    Pop-Location
}
