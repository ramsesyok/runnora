DECLARE
  v_cnt NUMBER;
BEGIN
  SELECT COUNT(*) INTO v_cnt FROM testuser.users;

  IF v_cnt = 0 THEN
    -- 明示的な例外送出でフック失敗を発生させるサンプル
    -- (通常の after hook では使わず、異常系テスト用)
    RAISE_APPLICATION_ERROR(-20001, 'No test data found: expected at least 1 row');
  END IF;

  DELETE FROM testuser.users;
  COMMIT;

EXCEPTION
  WHEN OTHERS THEN
    ROLLBACK;
    RAISE;
END;