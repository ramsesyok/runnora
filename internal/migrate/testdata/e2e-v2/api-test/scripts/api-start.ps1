# サンプル API (Go + Oracle) をビルドして前面で起動する (Ctrl+C で終了)。
# 先に scripts/db-up.ps1 で Oracle とスキーマを用意しておく。
. (Join-Path $PSScriptRoot '_common.ps1')
Push-Location $Root
try {
    go build -C api -o ../bin/library-api.exe .
    if ($LASTEXITCODE -ne 0) { throw 'API のビルドに失敗しました' }
} finally {
    Pop-Location
}
& (Join-Path $Root 'bin\library-api.exe')
