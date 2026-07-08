package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	userpb "latihan_auth/generated/go/user"
	deliverygrpc "latihan_auth/services/user-service/internal/delivery/grpc"
	"latihan_auth/services/user-service/internal/domain"
	"latihan_auth/services/user-service/internal/repository"
	"latihan_auth/services/user-service/internal/usecase"
)

func main() {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		getEnvOr("POSTGRES_HOST", "postgres"),
		mustEnv("POSTGRES_USER"),
		mustEnv("POSTGRES_PASSWORD"),
		mustEnv("POSTGRES_DB"), // database SENDIRI (user_db), beda dari auth-service (auth_db)
		getEnvOr("POSTGRES_PORT", "5432"),
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	if err := db.AutoMigrate(&domain.UserProfile{}); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	repo := repository.NewUserProfileRepository(db)
	uc := usecase.NewUserUsecase(repo)

	lis, err := net.Listen("tcp", ":"+getEnvOr("GRPC_PORT", "50052"))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	userpb.RegisterUserServiceServer(grpcServer, deliverygrpc.NewUserGRPCServer(uc))

	log.Printf("gRPC listening on %s", lis.Addr())
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %q is not set", key)
	}
	return v
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
