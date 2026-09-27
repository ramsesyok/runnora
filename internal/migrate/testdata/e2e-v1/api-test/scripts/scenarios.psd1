@{
    # シナリオ試験の一覧。test-api.ps1 (実行) と build-docs.ps1 (手順書生成) が共通で読む。
    #   Id        : 手順書の章・レポート名に使う ID
    #   Runbook   : 実行する runbook
    #   BeforeSql : --before-sql に渡すケース固有の前処理 (common.before の後に実行)
    #   AfterSql  : --after-sql に渡すケース固有の後処理 (common.after の前に実行)
    #   Expect    : 期待する runnora の終了コード (通常 0。検知デモは 4)
    Scenarios = @(
        @{ Id = 'lib-001'; Runbook = 'runbooks/scenarios/lib-001-loan-lifecycle.yml';   BeforeSql = @();                                          AfterSql = @('sql/cases/lib001_assert_after.sql');          Expect = 0 }
        @{ Id = 'lib-002'; Runbook = 'runbooks/scenarios/lib-002-no-stock.yml';         BeforeSql = @();                                          AfterSql = @();                                             Expect = 0 }
        @{ Id = 'lib-003'; Runbook = 'runbooks/scenarios/lib-003-overdue.yml';          BeforeSql = @('sql/cases/lib003_overdue_before.sql');     AfterSql = @();                                             Expect = 0 }
        @{ Id = 'lib-004'; Runbook = 'runbooks/scenarios/lib-004-loan-limit.yml';       BeforeSql = @('sql/cases/lib004_fill_loans_before.sql');  AfterSql = @();                                             Expect = 0 }
        @{ Id = 'lib-005'; Runbook = 'runbooks/scenarios/lib-005-savepoint.yml';        BeforeSql = @('sql/cases/lib005_savepoint_before.sql');   AfterSql = @('sql/cases/lib005_assert_after.sql');          Expect = 0 }
        @{ Id = 'lib-006'; Runbook = 'runbooks/scenarios/lib-006-validation-errors.yml'; BeforeSql = @();                                         AfterSql = @('sql/cases/lib006_assert_no_loans_after.sql'); Expect = 0 }
        @{ Id = 'lib-007'; Runbook = 'runbooks/scenarios/lib-007-bulk-history.yml';     BeforeSql = @('sql/cases/lib007_history_before.sql');     AfterSql = @();                                             Expect = 0 }
        @{ Id = 'demo-hook-failure'; Runbook = 'runbooks/demo/hook-failure-detection.yml'; BeforeSql = @();                                       AfterSql = @('sql/cases/demo_break_invariant_after.sql');   Expect = 4 }
    )
}
