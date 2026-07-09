# OAuth + SMTP — Simplified Notes

Versi ringkas dari [README.md](README.md). Kode lengkap cuma untuk 3 bagian yang paling sering dicontek pas quiz: **frontend**, **verifikasi id_token**, **handler endpoint**. Bagian lain (domain, repository, usecase orchestration, mailer) dijelasin alurnya doang — detail implementasi ada di file aslinya, ditunjuk lewat path.

---

## 1. Full code

### 1.1 Frontend

`frontend/src/pages/LoginPage.tsx` (potongan relevan):
```tsx
import { GoogleLogin, GoogleOAuthProvider } from '@react-oauth/google'
import { api, setToken } from '../lib/api'

const GOOGLE_CLIENT_ID = import.meta.env.VITE_GOOGLE_CLIENT_ID as string

// Dipanggil setelah Google berhasil kasih id_token ke browser.
async function handleGoogleSuccess(idToken: string) {
  const result = await api.loginGoogle(idToken)
  setToken(result.access_token) // simpan JWT, dipakai buat request ke /api/me di Dashboard
  navigate('/dashboard')
}

<GoogleOAuthProvider clientId={GOOGLE_CLIENT_ID}>
  {/* Provider inject context GIS ke child-nya, wajib membungkus <GoogleLogin> */}
  <GoogleLogin
    onSuccess={(res) => res.credential && handleGoogleSuccess(res.credential)}
    // res.credential = ID Token (JWT) hasil sign-in Google, BUKAN access_token OAuth biasa
    onError={() => setStatus('Google login failed')}
  />
</GoogleOAuthProvider>
```

`frontend/src/lib/api.ts` (potongan relevan):
```ts
async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(BASE + path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(getToken() ? { Authorization: `Bearer ${getToken()}` } : {}),
      ...options.headers,
    },
  })
  const data = await res.json().catch(() => null)
  if (!res.ok) throw new Error(data?.error || `Request failed with status ${res.status}`)
  return data as T
}

export const api = {
  loginGoogle: (idToken: string) =>
    request<AuthResult>('/auth/login/google', {
      method: 'POST',
      body: JSON.stringify({ id_token: idToken }), // kirim id_token mentah, backend yang verifikasi
    }),
}
```
(`forgotPassword`/`me` di `api.ts` pola-nya sama persis, cuma path & tipe return beda — lihat 2.4/2.5.)

### 1.2 Verifikasi id_token

`services/auth-service/internal/usecase/google.go`:
```go
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type googleIDTokenPayload struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Aud   string `json:"aud"`
}

func verifyGoogleIDToken(ctx context.Context, idToken, clientID string) (*googleIDTokenPayload, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://oauth2.googleapis.com/tokeninfo?id_token="+idToken, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google tokeninfo status %d", resp.StatusCode)
	}

	var p googleIDTokenPayload
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	if p.Sub == "" || p.Email == "" || p.Aud != clientID {
		return nil, fmt.Errorf("invalid google payload")
	}
	return &p, nil
}
```

### 1.3 Handler endpoint

`services/auth-service/internal/delivery/http/handler.go` (potongan relevan):
```go
// POST /api/auth/login/google
func (h *Handler) LoginGoogle(c *gin.Context) {
	var body struct {
		IDToken string `json:"id_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		// binding:"required" -> Gin otomatis reject kalau field kosong/gak ada
		c.JSON(http.StatusBadRequest, gin.H{"error": "id_token is required"})
		return
	}

	result, err := h.uc.LoginGoogle(c.Request.Context(), body.IDToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}
```
Registrasi di `router.go`:
```go
auth := api.Group("/auth")
auth.POST("/login/google", h.LoginGoogle)
```

Endpoint privat (`GET /api/me`) diproteksi middleware, yang validasi token-nya lewat usecase (bukan parse JWT langsung di sini):
```go
func (h *Handler) JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		claims, err := h.uc.ValidateToken(c.Request.Context(), strings.TrimPrefix(raw, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Next()
	}
}
```
(`ForgotPassword`, `VerifyOTP`, `Me` handler-nya pola sama — `ShouldBindJSON` lalu panggil 1 fungsi usecase — lihat 2.4.)

---

## 2. Sisanya — alur doang (bukan kode lengkap)

### 2.1 Domain layer
`services/auth-service/internal/domain/entity.go` & `repository.go`.
- Dua entity: `User` (id, email, username, google_id, role, is_verified) dan `OTPCode` (id, user_id, code, expires_at, used_at nullable).
- Dua *interface* repository dideklarasikan di sini (`UserRepository`, `OTPRepository`), berisi kontrak query yang dipakai usecase — usecase cuma bergantung ke interface ini, bukan ke GORM.

### 2.2 Repository layer
`services/auth-service/internal/repository/user_repository.go` & `otp_repository.go`.
- Implementasi konkret dari interface di atas pakai GORM.
- `UserRepository`: cari user by `google_id`, cari by `email`, create user baru, link `google_id` ke user existing. "Tidak ketemu" selalu dikembalikan sebagai `nil, nil` (bukan error) supaya caller cukup cek `user == nil`.
- `OTPRepository`: create OTP baru, ambil OTP terbaru milik user yang belum dipakai (`used_at IS NULL`, urut terbaru), tandai OTP sebagai used.

### 2.3 Usecase layer — `LoginGoogle`
`services/auth-service/internal/usecase/auth_usecase.go`.
1. Verifikasi `id_token` (panggil fungsi di 1.2) → dapat `sub`, `email`, `name`.
2. Cari user by `google_id` lewat `UserRepository`.
3. Kalau belum ketemu: cari lagi by `email`.
   - Belum ada juga → buat user baru (`IsVerified: true`, karena Google sudah verifikasi emailnya), lalu sinkron profil ke `user-service` lewat gRPC client (`userpb.UserServiceClient.CreateUser`) secara fire-and-forget (error diabaikan — auth-service tetap source of truth login, user-service cuma cache profil untuk service lain).
   - Sudah ada (daftar manual sebelumnya) → link `google_id` ke user itu (bukan bikin baru).
4. Issue token: JWT access token (klaim `user_id`+`email`, umur 15 menit, signed HS256) + refresh token random hex 32-byte (tidak disimpan ke DB di project ini). Return keduanya + data user.

### 2.4 Usecase layer — `ForgotPassword` & `VerifyOTP`
Fungsi sama di `auth_usecase.go`.
- **ForgotPassword**: cari user by email lewat `UserRepository`. Kalau tidak ketemu, tetap return sukses (supaya tidak bocorin email mana yang terdaftar). Kalau ketemu: generate OTP 6 digit (lihat 2.5), simpan lewat `OTPRepository.Create` dengan `expires_at` = now + 5 menit, lalu kirim email lewat `Mailer.Send`.
- **VerifyOTP**: cari user by email, ambil OTP terbaru-belum-dipakai lewat `OTPRepository.FindLatestUnused`, validasi belum expired dan kode cocok, lalu tandai `used_at` lewat `OTPRepository.MarkUsed` supaya tidak bisa dipakai ulang.
- **ValidateToken**: dipanggil `JWTMiddleware` (lihat 1.3) — parse & verifikasi signature JWT pakai `jwtSecret`, extract `user_id`/`email` dari klaim.

### 2.5 OTP generator
`services/auth-service/internal/usecase/otp.go` — `generateOTP()` pakai `crypto/rand` (bukan `math/rand`, supaya tidak predictable), ambil 3 byte random, gabung jadi angka, modulo sejuta, format `%06d` biar selalu 6 digit dengan leading zero.

### 2.6 Mailer
`services/auth-service/internal/mailer/smtp.go` — implementasi `Mailer` interface (kontraknya dideklarasikan di `usecase/auth_usecase.go`) pakai `net/smtp` bawaan Go. `Send(to, subject, htmlBody)` compose raw MIME message (header dipisah `\r\n`, baris kosong sebagai pemisah header/body) lalu kirim lewat `smtp.SendMail` dengan `PlainAuth`.

### 2.7 Wiring
`services/auth-service/cmd/main.go` — buka koneksi Postgres, `AutoMigrate` kedua entity, construct repository (inject `*gorm.DB`), construct mailer (inject config SMTP dari env), buka koneksi gRPC ke `user-service`, construct usecase (inject semua di atas + `jwtSecret` + `googleClientID`), construct handler (inject usecase), daftarkan route, jalankan server Gin.
