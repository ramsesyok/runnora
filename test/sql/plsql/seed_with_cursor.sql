DECLARE
  -- カーソルを使ってシードデータを INSERT する PL/SQL サンプル
  TYPE t_user IS RECORD (
    name  VARCHAR2(100),
    email VARCHAR2(255),
    age   NUMBER
  );
  TYPE t_user_list IS TABLE OF t_user INDEX BY PLS_INTEGER;

  v_users t_user_list;
  v_idx   PLS_INTEGER;
BEGIN
  -- シードデータをコレクションに積む
  v_users(1).name  := 'Seed User 1';
  v_users(1).email := 'seed1@example.com';
  v_users(1).age   := 20;

  v_users(2).name  := 'Seed User 2';
  v_users(2).email := 'seed2@example.com';
  v_users(2).age   := 21;

  v_users(3).name  := 'Seed User 3';
  v_users(3).email := 'seed3@example.com';
  v_users(3).age   := 22;

  -- FORALL で一括 INSERT
  v_idx := v_users.FIRST;
  WHILE v_idx IS NOT NULL LOOP
    INSERT INTO testuser.users (name, email, age)
    VALUES (v_users(v_idx).name, v_users(v_idx).email, v_users(v_idx).age);
    v_idx := v_users.NEXT(v_idx);
  END LOOP;

  COMMIT;
END;