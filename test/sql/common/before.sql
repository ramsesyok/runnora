BEGIN
  -- テスト前にデータをリセット (DDL は EXECUTE IMMEDIATE で発行)
  EXECUTE IMMEDIATE 'TRUNCATE TABLE testuser.users';
END;