DECLARE
  v_cnt  NUMBER;
BEGIN
  -- テスト後に残存レコード数を確認し、残っていれば警告を記録する
  SELECT COUNT(*) INTO v_cnt FROM testuser.users;

  IF v_cnt > 0 THEN
    -- アプリケーション固有エラーとして発行するとフック失敗になるため
    -- ここでは DBMS_OUTPUT に留める（監視用）
    DBMS_OUTPUT.PUT_LINE(
      'WARN: ' || v_cnt || ' row(s) remain in testuser.users after test run at '
      || TO_CHAR(SYSTIMESTAMP, 'YYYY-MM-DD HH24:MI:SS.FF3')
    );
  END IF;
END;