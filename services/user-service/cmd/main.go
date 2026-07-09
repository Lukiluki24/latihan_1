package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	userpb "latihan_auth/generated/go/user"
	deliverygrpc "latihan_auth/services/user-service/internal/delivery/grpc"
	deliveryhttp "latihan_auth/services/user-service/internal/delivery/http"
	"latihan_auth/services/user-service/internal/domain"
	"latihan_auth/services/user-service/internal/repository"
	"latihan_auth/services/user-service/internal/storage"
	"latihan_auth/services/user-service/internal/usecase"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func main() {
	ctx := context.Background()

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

	// blob storage (SeaweedFS, S3-compatible) buat avatar — avatar itu data
	// profil, jadi tempatnya di sini (user-service), bukan di auth-service.
	s3Bucket := mustEnv("SEAWEEDFS_BUCKET")
	s3Client := mustS3Client(ctx, mustEnv("SEAWEEDFS_S3_ENDPOINT"))
	ensureBucket(ctx, s3Client, s3Bucket)
	imageStorage := storage.New(s3Client, s3Bucket)

	uc := usecase.NewUserUsecase(repo, imageStorage)

	// HTTP delivery: satu-satunya permukaan publik user-service (upload/serve avatar).
	// Divalidasi JWT sendiri di sini (lihat Handler.JWTMiddleware), gak lewat
	// auth-service — makanya butuh JWT_SECRET yang sama dengan auth-service.
	go func() {
		gin.SetMode(getEnvOr("GIN_MODE", "release"))
		r := gin.New()
		h := deliveryhttp.NewHandler(uc, mustEnv("JWT_SECRET"))
		deliveryhttp.SetupRoutes(r, h)

		addr := ":" + getEnvOr("REST_PORT", "8081")
		log.Printf("HTTP listening on %s", addr)
		if err := r.Run(addr); err != nil {
			log.Fatalf("HTTP server: %v", err)
		}
	}()

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

// mustS3Client: arahkan SDK aws-sdk-go-v2 ke S3 gateway SeaweedFS (bukan AWS asli).
func mustS3Client(ctx context.Context, endpoint string) *s3.Client {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"), // wajib diisi non-kosong walau SeaweedFS gak peduli region
		// SeaweedFS jalan tanpa -s3.config di docker-compose (Allow-All Mode),
		// jadi access/secret key ini gak divalidasi — cukup isi apa aja.
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = &endpoint
		o.UsePathStyle = true // wajib true: SeaweedFS gak support virtual-hosted-style (bucket.host/key)
	})
}

// ensureBucket: SeaweedFS gak selalu auto-create bucket pas PutObject pertama,
// jadi lebih aman dipastikan eksplisit sekali di sini pas startup.
func ensureBucket(ctx context.Context, client *s3.Client, bucket string) {
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		log.Printf("create bucket %q: %v (abaikan kalau memang sudah ada)", bucket, err)
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
