# latihan_auth

Latihan Google OAuth 2.0 + SMTP verification code, infrastruktur & pola sama dengan project besar: **microservices** (auth-service + user-service via gRPC), Clean Architecture (`cmd → domain → repository → usecase → delivery`), GORM, Postgres (1 container, 2 database — tanpa master-slave/pgpool), nginx reverse proxy, JWT.

## Struktur
```
latihan_auth/
  go.mod                    — 1 module buat seluruh monorepo (semua service + generated code)
  docker-compose.yml        — postgres + auth-service + user-service + nginx
  .env.example              — copy jadi .env, isi credential asli
  proto/user/user.proto     — kontrak gRPC UserService
  generated/go/user/        — hasil `protoc` dari proto di atas
  services/
    auth-service/           — HTTP (Gin) + logic login Google, JWT, OTP/SMTP.
                               Manggil user-service via gRPC buat sinkronin profil.
    user-service/            — gRPC only, nyimpen profil user (email/username/role)
                               di database sendiri (user_db), terpisah dari auth-service (auth_db).
  frontend/                 — React + Vite, LoginPage (Google + forgot password) & DashboardPage
```

Kenapa dipecah gini: **auth-service** = source of truth buat kredensial & login (tabel `users` isinya google_id, is_verified, dst). **user-service** = "cache" profil yang dipakai fitur lain (bukan urusan login). Auth-service tetap yang nentuin bisa login atau nggak; user-service cuma disinkronin lewat gRPC `CreateUser` tiap ada user baru — persis pola `auth-service -> userpb.CreateUser` di project besar.

## Cara jalanin

1. Copy env & isi credential:
   ```
   cp .env.example .env
   ```
   - `GOOGLE_CLIENT_ID`: dari console.cloud.google.com → Credentials → OAuth client ID (Web application). Authorized JavaScript origin: `http://localhost`.
   - `SMTP_*`: pakai [Mailtrap](https://mailtrap.io) sandbox (gratis) buat testing kirim email, atau Gmail (butuh App Password).
   - `JWT_SECRET`: string random bebas.

2. Build & jalankan:
   ```
   docker compose up --build
   ```

3. Buka `http://localhost` — tombol Google login + form forgot password.

## Kalau ganti `GOOGLE_CLIENT_ID` setelah container sudah pernah dibuild

Vite bake env var ke bundle saat **build time**, jadi harus rebuild image `nginx`:
```
docker compose build nginx
docker compose up -d
```

## Kalau ubah proto (`proto/user/user.proto`)

Generate ulang kode Go-nya (butuh `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc` ter-install):
```
protoc --go_out=. --go_opt=module=latihan_auth \
       --go-grpc_out=. --go-grpc_opt=module=latihan_auth \
       proto/user/user.proto
```

## Endpoint (semua lewat auth-service, di-proxy nginx ke `/api/*`)

| Method | Path | Keterangan |
|---|---|---|
| POST | `/api/auth/login/google` | body `{ id_token }`, balikin JWT access+refresh token. Auth-service juga manggil `user-service.CreateUser` via gRPC di sini kalau user baru. |
| POST | `/api/auth/forgot-password` | body `{ email }`, generate OTP + kirim email (testing SMTP) |
| POST | `/api/auth/verify-otp` | body `{ email, code }` |
| GET | `/api/me` | proteksi JWT, header `Authorization: Bearer <access_token>` |

`user-service` gak expose HTTP/port ke host sama sekali — cuma bisa diakses lewat gRPC di dalam docker network (`user-service:50052`), persis kayak service internal di project besar.
