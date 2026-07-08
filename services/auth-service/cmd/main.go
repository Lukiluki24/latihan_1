package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	userpb "latihan_auth/generated/go/user"
	deliveryhttp "latihan_auth/services/auth-service/internal/delivery/http"
	"latihan_auth/services/auth-service/internal/domain"
	"latihan_auth/services/auth-service/internal/mailer"
	"latihan_auth/services/auth-service/internal/repository"
	"latihan_auth/services/auth-service/internal/usecase"
)

func main() {
	// postgres
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		getEnvOr("POSTGRES_HOST", "postgres"),
		mustEnv("POSTGRES_USER"),
		mustEnv("POSTGRES_PASSWORD"),
		mustEnv("POSTGRES_DB"),
		getEnvOr("POSTGRES_PORT", "5432"),
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}

	// AutoMigrate: bikin/update tabel otomatis sesuai struct domain, paling simpel buat latihan
	// (di project besar biasanya pakai file migration .sql manual per service).
	if err := db.AutoMigrate(&domain.User{}, &domain.OTPCode{}); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// repository (implementasi konkret dari interface domain.*Repository)
	userRepo := repository.NewUserRepository(db)
	otpRepo := repository.NewOTPRepository(db)

	// mailer (implementasi konkret dari interface usecase.Mailer)
	mail := mailer.New(mailer.Config{
		Host:     mustEnv("SMTP_HOST"),
		Port:     mustEnv("SMTP_PORT"),
		Username: mustEnv("SMTP_USERNAME"),
		Password: mustEnv("SMTP_PASSWORD"),
		From:     mustEnv("SMTP_FROM"),
	})

	// gRPC client ke user-service (service lain, container terpisah)
	userConn, err := grpc.NewClient(mustEnv("USER_SERVICE_ADDR"),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("user-service dial: %v", err)
	}
	defer userConn.Close()
	userClient := userpb.NewUserServiceClient(userConn)

	// usecase (cuma bergantung ke interface, gak tau GORM/SMTP/gRPC ada di baliknya)
	uc := usecase.NewAuthUsecase(
		userRepo,
		otpRepo,
		mail,
		userClient,
		mustEnv("JWT_SECRET"),
		mustEnv("GOOGLE_CLIENT_ID"),
	)

	// HTTP delivery
	gin.SetMode(getEnvOr("GIN_MODE", "release"))
	r := gin.New()
	h := deliveryhttp.NewHandler(uc)
	deliveryhttp.SetupRoutes(r, h)

	addr := ":" + getEnvOr("PORT", "8080")
	log.Printf("HTTP listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("HTTP server: %v", err)
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
