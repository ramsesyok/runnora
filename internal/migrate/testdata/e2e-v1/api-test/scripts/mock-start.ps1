# WireMock を前面で起動する (Ctrl+C で終了)。先に scripts/mock-build.ps1 を実行しておく。
# jar の場所は環境変数 WIREMOCK_JAR で変更できる (既定: ..\..\runnora\docs\tools\wiremock-standalone-3.13.2.jar)。
. (Join-Path $PSScriptRoot '_common.ps1')
$jar = Resolve-Tool 'WIREMOCK_JAR' (Join-Path $Projects 'runnora\docs\tools\wiremock-standalone-3.13.2.jar') $null
Write-Host "WireMock: $MockUrl (root: mock/wiremock-out)"
& java -jar $jar --port 18080 --bind-address 127.0.0.1 --root-dir (Join-Path $Root 'mock\wiremock-out') --disable-banner
