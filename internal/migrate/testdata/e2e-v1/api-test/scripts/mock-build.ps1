# OpenAPI + mock/mock-cases.yaml + fixtures/responses/ から WireMock 資産を生成する (oapi2wire)。
#   validate : case YAML と OpenAPI の整合性を検査 (不整合は exit 1)
#   build    : mock/wiremock-out/{mappings,__files} を作り直す
. (Join-Path $PSScriptRoot '_common.ps1')
Push-Location $Root
try {
    & $Oapi2wire validate --openapi openapi/library-api.yaml --cases mock/mock-cases.yaml --responses-root fixtures/responses
    if ($LASTEXITCODE -ne 0) { throw 'oapi2wire validate に失敗しました' }
    # 契約テストの期待本文 (suite の vars.expected) とモックの bodyFile が同じファイルを指しているか検査する
    go build -C tools -o ../bin/contract-check.exe ./contract-check
    if ($LASTEXITCODE -ne 0) { throw 'contract-check のビルドに失敗しました' }
    & (Join-Path $Root 'bin\contract-check.exe') -root $Root
    if ($LASTEXITCODE -ne 0) { throw '契約ケースとモックの対応に不整合があります' }
    & $Oapi2wire build --openapi openapi/library-api.yaml --cases mock/mock-cases.yaml --responses-root fixtures/responses `
        --out mock/wiremock-out --clean --fail-on-missing-operation --fail-on-missing-body-file
    if ($LASTEXITCODE -ne 0) { throw 'oapi2wire build に失敗しました' }
} finally {
    Pop-Location
}
