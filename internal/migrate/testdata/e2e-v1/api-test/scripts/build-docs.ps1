# 手順書 (docs/) の生成原稿を作り、ddq で HTML / PDF を発行する。
#   1. tools/openapi-doc : OpenAPI → docs/generated/api/api-spec.qmd
#   2. runnora-docgen    : 契約 suite → docs/generated/contract/<suite>/
#                          シナリオ + 前後処理 SQL → docs/generated/scenarios/<runbook>/
#   3. 章から include する取り込み原稿 (_body.qmd) を作る
#   4. ddq html / pdf
# ツールの場所: 環境変数 RUNNORA_DOCGEN_EXE / DDQ_EXE で変更できる。
param(
    [ValidateSet('Html', 'Pdf', 'Both', 'None')]
    [string]$Format = 'Html'
)
. (Join-Path $PSScriptRoot '_common.ps1')
$docgen = Resolve-Tool 'RUNNORA_DOCGEN_EXE' (Join-Path $Projects 'runnora-docgen\runnora-docgen.exe') 'runnora-docgen'
$defs = Import-PowerShellDataFile (Join-Path $PSScriptRoot 'scenarios.psd1')
$utf8 = New-Object System.Text.UTF8Encoding($false)

# 章ファイル (docs/chapters/<章>/index.qmd) から見た相対パスで include を並べる。
# ddq の book では入れ子の include も「章ファイルのディレクトリ」基準で解決されるため、
# 生成原稿を ../../generated/<Sub>/ で指す。
function Write-Body([string]$Sub, [string]$Heading, [switch]$NoHooks) {
    $GeneratedDir = Join-Path $Root "docs\generated\$Sub"
    $sections = @(
        @('scenario.qmd', 'テストシナリオ'), @('cases.qmd', 'ケースデータ'), @('before.qmd', '前処理'),
        @('after.qmd', '後処理'), @('http.qmd', 'HTTP 呼び出し'), @('request-json.qmd', 'リクエストボディ'),
        @('expectations.qmd', '期待値・検証条件')
    )
    if ($NoHooks) { $sections = @($sections | Where-Object { $_[0] -notin @('before.qmd', 'after.qmd') }) }
    $lines = @('<!-- scripts/build-docs.ps1 が生成する。手で編集しない。 -->', '')
    foreach ($s in $sections) {
        if (Test-Path -LiteralPath (Join-Path $GeneratedDir $s[0])) {
            $rel = "../../generated/$Sub/$($s[0])"
            $lines += @("$Heading $($s[1])", '', "{{< include $rel >}}", '')
        }
    }
    [IO.File]::WriteAllLines((Join-Path $GeneratedDir '_body.qmd'), $lines, $utf8)
}

Push-Location $Root
try {
    $gen = Join-Path $Root 'docs\generated'
    Remove-DirectoryWithRetry $gen

    Write-Host '== API 仕様 (tools/openapi-doc)'
    go build -C tools -o ../bin/openapi-doc.exe ./openapi-doc
    if ($LASTEXITCODE -ne 0) { throw 'openapi-doc のビルドに失敗しました' }
    & (Join-Path $Root 'bin\openapi-doc.exe') -in openapi/library-api.yaml -out docs/generated/api/api-spec.qmd
    if ($LASTEXITCODE -ne 0) { throw 'API 仕様の生成に失敗しました' }

    Write-Host '== 契約テスト (runnora-docgen)'
    $contract = @(Get-ChildItem runbooks/contract -Filter *.suite.yml | Sort-Object Name | ForEach-Object { "runbooks/contract/$($_.Name)" })
    & $docgen generate --base-dir . --config config.yaml --before-sql sql/cases/contract_setup.sql --out docs/generated/contract --force @contract
    if ($LASTEXITCODE -ne 0) { throw '契約テストの原稿生成に失敗しました' }
    # 前処理・後処理は全 suite で同一なので、先頭の suite の表を 1 回だけ載せる
    $first = [IO.Path]::GetFileNameWithoutExtension($contract[0]).Replace('_', '-').Replace('.', '-').ToLower()
    $index = @('<!-- scripts/build-docs.ps1 が生成する。手で編集しない。 -->', '',
        '## 前処理・後処理（全 suite 共通）', '',
        "{{< include ../../generated/contract/$first/before.qmd >}}", '',
        "{{< include ../../generated/contract/$first/after.qmd >}}", '')
    foreach ($suite in $contract) {
        $name = [IO.Path]::GetFileNameWithoutExtension($suite).Replace('_', '-').Replace('.', '-').ToLower()
        Write-Body "contract/$name" '###' -NoHooks
        $desc = (Select-String -LiteralPath $suite -Pattern '^desc:\s*(.+)$' -Encoding UTF8).Matches[0].Groups[1].Value
        $rel = "../../generated/contract/$name/_body.qmd"
        $index += @("## $desc", '', "出典：``$suite``", '', "{{< include $rel >}}", '')
    }
    [IO.File]::WriteAllLines((Join-Path $gen 'contract\_index.qmd'), $index, $utf8)

    Write-Host '== 付録: 前後処理 SQL 全文'
    # runnora-docgen の前処理・後処理表はファイル名だけを示すので、SQL 全文を付録にコードブロックで掲載する
    $listing = @('<!-- scripts/build-docs.ps1 が生成する。手で編集しない。 -->', '')
    foreach ($dir in @('common', 'cases')) {
        $listing += @("## sql/$dir/", '')
        foreach ($file in Get-ChildItem "sql/$dir" -Filter *.sql | Sort-Object Name) {
            $listing += @("### $($file.Name) {#sec-sql-$dir-$($file.BaseName.Replace('_', '-'))}", '', '```sql')
            $listing += [IO.File]::ReadAllLines($file.FullName, [Text.Encoding]::UTF8)
            $listing += @('```', '')
        }
    }
    New-Item -ItemType Directory -Force (Join-Path $gen 'sql') | Out-Null
    [IO.File]::WriteAllLines((Join-Path $gen 'sql\sql-listing.qmd'), $listing, $utf8)

    Write-Host '== シナリオ試験 (runnora-docgen)'
    foreach ($s in $defs.Scenarios) {
        $dgArgs = @('generate', '--base-dir', '.', '--config', 'config.yaml', '--out', 'docs/generated/scenarios', '--force')
        foreach ($f in $s.BeforeSql) { $dgArgs += @('--before-sql', $f) }
        foreach ($f in $s.AfterSql) { $dgArgs += @('--after-sql', $f) }
        & $docgen @dgArgs $s.Runbook
        if ($LASTEXITCODE -ne 0) { throw "$($s.Id) の原稿生成に失敗しました" }
        $name = [IO.Path]::GetFileNameWithoutExtension($s.Runbook)
        Write-Body "scenarios/$name" '###'
    }

    if ($Format -eq 'None') { return }
    $ddq = Resolve-Tool 'DDQ_EXE' '' 'ddq'
    if (-not (Test-Path -LiteralPath 'docs/_quarto-publish.yml') -and $Format -in @('Pdf', 'Both')) {
        Write-Host '(初回の PDF 発行では ddq が PDF 用の機構ファイルを docs/ に置きます)'
    }
    if ($Format -in @('Pdf', 'Both')) {
        & $ddq pdf docs
        if ($LASTEXITCODE -ne 0) { throw 'ddq pdf に失敗しました' }
    }
    if ($Format -in @('Html', 'Both')) {
        & $ddq html docs
        if ($LASTEXITCODE -ne 0) { throw 'ddq html に失敗しました' }
        Write-Host "HTML: $(Join-Path $Root 'docs\_book\index.html')"
    }
} finally {
    Pop-Location
}
