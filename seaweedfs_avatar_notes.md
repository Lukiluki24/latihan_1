# Cheatsheet: Avatar Upload dengan SeaweedFS (blob storage)

Fitur avatar upload di project ini hidup di **`user-service`**, bukan `auth-service` — walau endpoint-nya butuh JWT (yang di-issue `auth-service`). Alasannya: avatar itu **data profil**, bukan **data autentikasi**. `auth-service` cuma boleh tau soal identitas & token; apapun yang sifatnya "profil" (username, bio, avatar) tempatnya di `user-service`. Kalau kamu ngikutin cheatsheet ini terus taruh semua logic di `auth-service` biar gampang — itu jalan pintas, bukan pola yang benar (sempat kejadian di iterasi awal project ini, lihat § 7).

---

## 1. Alur

```
Browser PUT multipart file -> nginx -> user-service (BUKAN lewat auth-service)
-> user-service validasi JWT SENDIRI (secret sama kayak auth-service)
-> validasi tipe file (sniff byte asli) & ukuran -> PutObject ke SeaweedFS
-> simpan key ke kolom avatar_url -> return path publik
```

Kenapa `user-service` bisa validasi JWT sendiri padahal yang issue token itu `auth-service`? Karena JWT itu **stateless** — signature-nya diverifikasi pakai `JWT_SECRET` yang sama, gak perlu nanya balik ke `auth-service` tiap request (beda dari session-based auth yang wajib cek ke server pemilik sesi).

---

## 2. SeaweedFS (docker-compose)

Cukup 1 container: `weed server -s3` gabungin master+volume+filer+s3-gateway jadi 1 proses.

```yaml
seaweedfs:
  image: chrislusf/seaweedfs:latest
  command: server -s3 -dir=/data
  volumes:
    - seaweedfs-data:/data
```
Auth di-skip (gak ada flag `-s3.config`) → SeaweedFS jalan di **"Allow-All Mode"**: semua request S3 diizinkan tanpa autentikasi. Aman selama container ini gak pernah expose port ke host (di sini memang gak di-`ports:`, cuma reachable lewat nama service di docker network).

`user-service` env:
```
SEAWEEDFS_S3_ENDPOINT=http://seaweedfs:8333
SEAWEEDFS_BUCKET=avatars
JWT_SECRET=<sama persis dengan punya auth-service>
```

---

## 3. Kode

### 3.1 Domain — `services/user-service/internal/domain/entity.go`
```go
type UserProfile struct {
	UserID    string `gorm:"column:user_id;primaryKey"`
	Email     string
	Username  string
	Role      string
	AvatarURL string // key di blob storage, mis. "avatars/<id>/avatar.jpg" — bukan URL penuh
}
```
`domain/repository.go` nambah 1 method ke interface:
```go
type UserProfileRepository interface {
	Create(ctx context.Context, p *UserProfile) error
	FindByID(ctx context.Context, userID string) (*UserProfile, error)
	+UpdateAvatarURL(ctx context.Context, userID, key string) error
}
```

### 3.2 Storage layer — `services/user-service/internal/storage/s3.go`
```go
type S3ImageStorage struct {
	client *s3.Client
	bucket string
}

func New(client *s3.Client, bucket string) *S3ImageStorage {
	return &S3ImageStorage{client: client, bucket: bucket}
}

// Key dibentuk pemanggil (deterministik: "avatars/<id>/avatar.jpg") supaya
// upload ulang otomatis menimpa file lama — gak perlu hapus manual dulu.
func (s *S3ImageStorage) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
		Body: bytes.NewReader(data), ContentType: aws.String(contentType),
	})
	return err
}

func (s *S3ImageStorage) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, "", err
	}
	contentType := "application/octet-stream"
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	return out.Body, contentType, nil
}
```
`usecase/storage.go` deklarasiin interface-nya (dependency inversion, usecase gak tau ini S3/SeaweedFS):
```go
type ImageStorage interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, string, error)
}

const maxAvatarBytes = 2 * 1024 * 1024 // 2MB
var allowedAvatarTypes = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}
```

### 3.3 Usecase — `services/user-service/internal/usecase/user_usecase.go`
```go
// Tipe file di-deteksi dari ISI byte-nya (http.DetectContentType), BUKAN dari
// header Content-Type yang diklaim client — client bisa bohong (mis. upload
// file .html tapi ngaku "image/jpeg" biar lolos whitelist).
func (uc *userUsecase) UpdateAvatar(ctx context.Context, userID string, data []byte) (string, error) {
	if len(data) > maxAvatarBytes {
		return "", errors.New("file too large (max 2MB)")
	}
	sniffed := http.DetectContentType(data)
	ext, ok := allowedAvatarTypes[sniffed]
	if !ok {
		return "", errors.New("only jpg/png/webp allowed")
	}

	key := fmt.Sprintf("avatars/%s/avatar.%s", userID, ext)
	if err := uc.storage.Upload(ctx, key, data, sniffed); err != nil {
		return "", err
	}
	if err := uc.repo.UpdateAvatarURL(ctx, userID, key); err != nil {
		return "", err
	}
	return key, nil
}

func (uc *userUsecase) GetAvatar(ctx context.Context, userID, filename string) (io.ReadCloser, string, error) {
	return uc.storage.Get(ctx, fmt.Sprintf("avatars/%s/%s", userID, filename))
}
```

### 3.4 Handler + JWT self-validation — `services/user-service/internal/delivery/http/handler.go`
```go
type Handler struct {
	uc        usecase.UserUsecase
	jwtSecret string
}

func (h *Handler) UpdateAvatar(c *gin.Context) {
	file, _, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "avatar file is required"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 4*1024*1024))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	key, err := h.uc.UpdateAvatar(c.Request.Context(), c.GetString("user_id"), data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"avatar_url": "/api/users/media/" + key})
}

func (h *Handler) GetAvatar(c *gin.Context) {
	body, contentType, err := h.uc.GetAvatar(c.Request.Context(), c.Param("userId"), c.Param("filename"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "avatar not found"})
		return
	}
	defer body.Close()
	c.Header("X-Content-Type-Options", "nosniff") // cegah browser nebak-nebak ulang tipe konten
	c.DataFromReader(http.StatusOK, -1, contentType, body, nil)
}

// user-service verifikasi JWT SENDIRI (stateless) — secret & shape claim
// ("user_id", "email") harus sama persis dengan yang di-issue auth-service.
func (h *Handler) JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		claims := jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(strings.TrimPrefix(raw, "Bearer "), claims, func(t *jwt.Token) (interface{}, error) {
			return []byte(h.jwtSecret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		userID, _ := claims["user_id"].(string)
		c.Set("user_id", userID)
		c.Next()
	}
}
```

### 3.5 Router — `services/user-service/internal/delivery/http/router.go`
```go
func SetupRoutes(r *gin.Engine, h *Handler) {
	r.Use(gin.Recovery())
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	// Grup /api/users supaya nginx bisa bedain rute ini dari punya auth-service.
	api := r.Group("/api/users")
	{
		api.PUT("/me/avatar", h.JWTMiddleware(), h.UpdateAvatar)
		api.GET("/media/avatars/:userId/:filename", h.GetAvatar)
	}
}
```

### 3.6 Wiring — `services/user-service/cmd/main.go`
`user-service` sekarang jalanin **2 server sekaligus**: gRPC (buat `CreateUser`/`GetUser` dari `auth-service`, kayak sebelumnya) + HTTP baru (buat avatar). HTTP dijalanin di goroutine, gRPC blocking di main:
```go
uc := usecase.NewUserUsecase(repo, imageStorage)

go func() {
	r := gin.New()
	h := deliveryhttp.NewHandler(uc, mustEnv("JWT_SECRET"))
	deliveryhttp.SetupRoutes(r, h)
	r.Run(":" + getEnvOr("REST_PORT", "8081"))
}()

lis, _ := net.Listen("tcp", ":"+getEnvOr("GRPC_PORT", "50052"))
grpcServer := grpc.NewServer()
userpb.RegisterUserServiceServer(grpcServer, deliverygrpc.NewUserGRPCServer(uc))
grpcServer.Serve(lis)
```
S3 client + `ensureBucket()` (bikin bucket eksplisit pas startup, karena SeaweedFS gak selalu auto-create bucket pas `PutObject` pertama) — sama persis polanya kayak yang biasa dipakai di `auth-service` buat resource lain (bandingin ke `mailer.New(...)` di sana).

### 3.7 nginx — dua tujuan proxy sekaligus
```nginx
# /api/users/* ke user-service (avatar)
location /api/users/ {
    proxy_pass http://user-service:8081/api/users/;
    proxy_set_header Host $host;
}

# sisanya /api/* ke auth-service (login, OTP, /me)
location /api/ {
    proxy_pass http://auth-service:8080/api/;
    proxy_set_header Host $host;
}
```
nginx otomatis milih *prefix location* terpanjang yang cocok, jadi `/api/users/...` ketangkep block pertama walau ada block `/api/` yang lebih umum di bawahnya — urutan penulisan gak berpengaruh buat prefix location biasa (beda kalau pakai regex `location ~`).

### 3.8 Frontend — `frontend/src/lib/api.ts`
```ts
uploadAvatar: async (file: File) => {
  const form = new FormData()
  form.append('avatar', file)
  const res = await fetch(BASE + '/users/me/avatar', {
    method: 'PUT',
    headers: getToken() ? { Authorization: `Bearer ${getToken()}` } : {},
    body: form,
  })
  const data = await res.json().catch(() => null)
  if (!res.ok) throw new Error(data?.error || `Request failed with status ${res.status}`)
  return data as { avatar_url: string }
}
```
Multipart upload sengaja gak lewat wrapper `request()` biasa — kalau kita paksa header `Content-Type: application/json`, browser gak akan nge-set `boundary` multipart yang benar. `DashboardPage.tsx` panggil ini dari `<input type="file">`, lalu langsung `<img src={avatarUrl}>` pakai hasilnya (gak perlu refresh `/api/me`).

---