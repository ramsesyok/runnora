# runnora generate が作った template に、応答保存と独立した判定を加える。
# generated/ は再生成で上書きされるため、generate.ps1 から毎回呼ぶ。
$ErrorActionPreference = 'Stop'
$generated = Join-Path (Split-Path $PSScriptRoot -Parent) 'runbooks\generated'

foreach ($template in Get-ChildItem $generated -Recurse -Filter '*.template.yml') {
    $operation = $template.BaseName -replace '\.template$', ''
    $source = [IO.File]::ReadAllText($template.FullName)
    $pattern = '(?m)^    test: \|\r?\n      current\.res\.status == vars\.case\.expect\.status\s*$'
    if ([regex]::Matches($source, $pattern).Count -ne 1) {
        throw "生成 template の判定を特定できません: $($template.FullName)"
    }
    $replacement = @"
  dump_response:
    dump:
      expr: steps.call_api.res
      out: '{{ env.RUNNORA_EVIDENCE_DIR }}/$operation-{{ vars.case.name }}.json'
  check_response:
    test: steps.call_api.res.status == vars.case.expect.status
"@
    $updated = [regex]::Replace($source, $pattern, "`n$replacement")
    [IO.File]::WriteAllText($template.FullName, $updated.TrimEnd() + "`n", [Text.UTF8Encoding]::new($false))
}
