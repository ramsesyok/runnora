package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "github.com/ramsesyok/runnora-testgrpc/proto/userv1"
	_ "github.com/sijms/go-ora/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// --- DB ---

var db *sql.DB

func openDB() (*sql.DB, error) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "oracle://testuser:TestPass1!@localhost:1521/FREEPDB1"
	}
	d, err := sql.Open("oracle", dsn)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(5)
	d.SetMaxIdleConns(2)
	d.SetConnMaxLifetime(5 * time.Minute)
	return d, nil
}

// --- Server ---

type userServer struct {
	pb.UnimplementedUserServiceServer
}

func (s *userServer) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.User, error) {
	if req.Name == "" || req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "name and email are required")
	}

	_, err := db.ExecContext(ctx,
		`INSERT INTO testuser.users (name, email, age) VALUES (:1, :2, :3)`,
		req.Name, req.Email, nullableAge(req.Age))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, status.Error(codes.AlreadyExists, "email already exists")
		}
		return nil, status.Errorf(codes.Internal, "insert: %v", err)
	}

	return s.fetchByEmail(ctx, req.Email)
}

func (s *userServer) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.User, error) {
	u, err := s.fetchByID(ctx, req.Id)
	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "user %d not found", req.Id)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return u, nil
}

func (s *userServer) ListUsers(ctx context.Context, _ *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, email, age, created_at FROM testuser.users ORDER BY id`)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	defer rows.Close()

	var users []*pb.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "%v", err)
		}
		users = append(users, u)
	}
	return &pb.ListUsersResponse{Users: users}, nil
}

func (s *userServer) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.User, error) {
	exists, err := s.userExists(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	if !exists {
		return nil, status.Errorf(codes.NotFound, "user %d not found", req.Id)
	}

	if req.Name != "" {
		if _, err := db.ExecContext(ctx,
			`UPDATE testuser.users SET name = :1 WHERE id = :2`, req.Name, req.Id); err != nil {
			return nil, status.Errorf(codes.Internal, "%v", err)
		}
	}
	if req.Email != "" {
		if _, err := db.ExecContext(ctx,
			`UPDATE testuser.users SET email = :1 WHERE id = :2`, req.Email, req.Id); err != nil {
			return nil, status.Errorf(codes.Internal, "%v", err)
		}
	}
	if req.Age != 0 {
		if _, err := db.ExecContext(ctx,
			`UPDATE testuser.users SET age = :1 WHERE id = :2`, req.Age, req.Id); err != nil {
			return nil, status.Errorf(codes.Internal, "%v", err)
		}
	}
	return s.fetchByID(ctx, req.Id)
}

func (s *userServer) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	res, err := db.ExecContext(ctx,
		`DELETE FROM testuser.users WHERE id = :1`, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, status.Errorf(codes.NotFound, "user %d not found", req.Id)
	}
	return &pb.DeleteUserResponse{Success: true}, nil
}

// --- helpers ---

func (s *userServer) fetchByEmail(ctx context.Context, email string) (*pb.User, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, name, email, age, created_at FROM testuser.users WHERE email = :1`, email)
	return scanUser(row)
}

func (s *userServer) fetchByID(ctx context.Context, id int64) (*pb.User, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, name, email, age, created_at FROM testuser.users WHERE id = :1`, id)
	return scanUser(row)
}

func (s *userServer) userExists(ctx context.Context, id int64) (bool, error) {
	var cnt int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM testuser.users WHERE id = :1`, id).Scan(&cnt)
	return cnt > 0, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(s scanner) (*pb.User, error) {
	var u pb.User
	var age sql.NullInt32
	var createdAt time.Time
	if err := s.Scan(&u.Id, &u.Name, &u.Email, &age, &createdAt); err != nil {
		return nil, err
	}
	if age.Valid {
		u.Age = age.Int32
	}
	u.CreatedAt = createdAt.Format(time.RFC3339)
	return &u, nil
}

func nullableAge(age int32) any {
	if age == 0 {
		return nil
	}
	return age
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for i := 0; i <= len(msg)-9; i++ {
		if msg[i:i+9] == "ORA-00001" {
			return true
		}
	}
	return false
}

// --- Main ---

func main() {
	var err error
	db, err = openDB()
	if err != nil {
		log.Fatalf("DB open: %v", err)
	}
	defer db.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterUserServiceServer(srv, &userServer{})
	reflection.Register(srv) // サーバーリフレクションを有効化 (runn がプロトファイル不要になる)

	log.Printf("testgrpc listening on :%s", port)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
