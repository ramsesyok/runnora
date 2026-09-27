# OpenAPI からテストの雛形 (runbooks/generated/, cases/generated/) を作り直す。
# 生成後に応答証跡と独立した判定を追加する。育てるテストは runbooks/contract/ に複製して使う。
. (Join-Path $PSScriptRoot '_common.ps1')
Push-Location $Root
try {
    # 出力先は runnora.yaml のあるフォルダ。template の接続先は ${RUNNORA_BASE_URL} で、実行時に環境 (unit / mock) の vars で決まる
    & $Runnora generate --openapi openapi/library-api.yaml --clean --force
    if ($LASTEXITCODE -ne 0) { throw 'runnora generate に失敗しました' }
    & (Join-Path $PSScriptRoot 'add-response-evidence.ps1')
} finally {
    Pop-Location
}
