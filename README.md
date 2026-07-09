# Cheatsheet: Google OAuth 2.0 + SMTP Verification Code

Stack di project ini: Go (Gin untuk `auth-service`, gRPC untuk `user-service`), GORM + Postgres, Clean Architecture (`domain` → `repository` → `usecase` → `delivery`), React+Vite frontend. Snippet di bawah diambil langsung dari kode di repo ini (bukan contoh generik) — kalau kelompokmu pakai schema/nama beda, tinggal sesuaikan, pola/urutannya tetap sama.

```bash
go get gorm.io/gorm gorm.io/driver/postgres
```
```go
// dsn (data source name) = string koneksi ke Postgres: host, user, password, nama db, port
dsn := "host=localhost user=postgres password=postgres dbname=esdemy port=5432 sslmode=disable"
db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{}) // buka koneksi + wrap jadi *gorm.DB, dipakai di semua fungsi repo bawah
```

---

## 1. Google OAuth 2.0

### 1.1 Alur
```
Browser klik tombol -> Google balikin id_token (JWT) -> Browser POST id_token ke backend
-> Backend verifikasi id_token ke Google -> Backend cari/buat user -> Backend issue JWT sendiri
```
`client_secret` **tidak dipakai** di pola ini (popup/One Tap) karena tidak ada tahap tukar `code` di server. Baru kepake kalau pakai redirect flow (lihat 1.8).

### 1.2 Google Cloud Console
1. console.cloud.google.com → buat project → **OAuth consent screen** (scope `email profile openid` — ini yang nentuin data apa yang boleh diminta dari akun Google user).
2. **Credentials → Create Credentials → OAuth client ID** → tipe **Web application** (bukan "Desktop"/"Android", karena kita web app).
3. **Authorized JavaScript origins**: `http://localhost:5173` — origin frontend yang diizinkan manggil GIS SDK; kalau beda origin, Google akan reject request-nya.
4. Copy **Client ID** (format `xxxx.apps.googleusercontent.com`) → simpan sebagai env var `GOOGLE_CLIENT_ID`, dipakai di frontend & backend.

### 1.3 Frontend
```bash
npm install @react-oauth/google
```
Library resmi wrapper React di atas GIS JS SDK Google — otomatis handle load script, render tombol, dan popup sign-in, tanpa kita perlu nulis `<script>` manual.

`frontend/src/pages/LoginPage.tsx`:
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

`frontend/src/lib/api.ts` — request tidak dipanggil `fetch` langsung di komponen, tapi lewat wrapper yang otomatis nempelin header & bongkar error:
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
`.env`: `VITE_GOOGLE_CLIENT_ID=xxxx.apps.googleusercontent.com` — prefix `VITE_` wajib, tanpa itu Vite tidak akan expose variabel ke bundle browser.

### 1.4 Backend — verifikasi id_token
`services/auth-service/internal/usecase/google.go`:
```go
// Field JSON tag harus PERSIS sama nama field yang dibalikin Google, kalau salah ketik hasilnya kosong bukan error
type googleIDTokenPayload struct {
	Sub   string `json:"sub"`   // Google User ID unik permanen — pakai ini sebagai identifier, JANGAN email (email bisa ganti)
	Email string `json:"email"`
	Name  string `json:"name"`
	Aud   string `json:"aud"`   // audience = client_id tujuan token ini dibuat, wajib dicek biar token dari app Google lain ditolak
}

func verifyGoogleIDToken(ctx context.Context, idToken, clientID string) (*googleIDTokenPayload, error) {
	// GET ke endpoint publik Google, id_token dikirim sebagai query param
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://oauth2.googleapis.com/tokeninfo?id_token="+idToken, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // wajib ditutup, kalau lupa -> connection leak

	if resp.StatusCode != http.StatusOK {
		// Google balikin non-200 kalau token invalid/sudah expired/salah format
		return nil, fmt.Errorf("google tokeninfo status %d", resp.StatusCode)
	}

	var p googleIDTokenPayload
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	if p.Sub == "" || p.Email == "" || p.Aud != clientID {
		// aud tidak cocok = token ini dibuat untuk client Google App LAIN, jangan diterima
		return nil, fmt.Errorf("invalid google payload")
	}
	return &p, nil
}
```
Tidak butuh library eksternal, cukup `net/http` + `encoding/json` bawaan Go. Fungsi & struct sengaja `unexported` (huruf kecil) karena cuma dipakai internal di package `usecase`.

### 1.5 Data layer + usecase (Clean Architecture, GORM)

Beda dari pola "1 fungsi 1 file" yang sering dipakai pas quiz, project ini pisah tegas 3 layer supaya usecase gak bergantung ke GORM langsung:

- **`domain/entity.go`** — struct data murni (`User`, `OTPCode`) + tag `gorm` buat mapping kolom.
- **`domain/repository.go`** — *interface* kontrak (`UserRepository`, `OTPRepository`). Usecase cuma kenal interface ini.
- **`repository/user_repository.go`**, **`repository/otp_repository.go`** — implementasi konkret pakai GORM, di-inject ke usecase lewat constructor (dependency inversion).

```go
// domain/entity.go
type User struct {
	ID         string `gorm:"primaryKey"`
	Email      string `gorm:"uniqueIndex"`
	Username   string
	GoogleID   string `gorm:"column:google_id;uniqueIndex"`
	Role       string
	IsVerified bool
	CreatedAt  time.Time
}

// domain/repository.go
type UserRepository interface {
	FindByGoogleID(ctx context.Context, googleID string) (*User, error) // nil,nil kalau gak ketemu
	FindByEmail(ctx context.Context, email string) (*User, error)
	Create(ctx context.Context, u *User) error
	LinkGoogleID(ctx context.Context, userID, googleID string) error
}
```

```go
// repository/user_repository.go — implementasi konkret dari domain.UserRepository
type userRepository struct{ db *gorm.DB }

func (r *userRepository) FindByGoogleID(ctx context.Context, googleID string) (*domain.User, error) {
	var u domain.User
	err := r.db.WithContext(ctx).Where("google_id = ?", googleID).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil // gak ketemu -> nil,nil (bukan error) supaya caller gampang cek `user == nil`
	}
	return &u, err
}
// FindByEmail, Create, LinkGoogleID: pola sama (lihat file aslinya)
```

Usecase (`usecase/auth_usecase.go`) meng-orkestrasi langkah-langkahnya:

```go
func (uc *authUsecase) LoginGoogle(ctx context.Context, idToken string) (*AuthResult, error) {
	// Langkah 1: pastikan id_token ini valid & memang dari Google
	payload, err := verifyGoogleIDToken(ctx, idToken, uc.googleClientID)
	if err != nil {
		return nil, fmt.Errorf("google token invalid: %w", err)
	}

	// Langkah 2: sudah pernah login Google sebelumnya?
	user, err := uc.userRepo.FindByGoogleID(ctx, payload.Sub)
	if err != nil {
		return nil, err
	}

	if user == nil {
		// Langkah 3: belum pernah — cek apakah emailnya sudah terdaftar (daftar manual)
		user, err = uc.userRepo.FindByEmail(ctx, payload.Email)
		if err != nil {
			return nil, err
		}
		if user == nil {
			// Langkah 4a: user benar-benar baru -> auto-register, langsung verified
			// (IsVerified: true karena Google sudah memverifikasi emailnya, gak perlu OTP lagi)
			user = &domain.User{ID: newID(), Email: payload.Email, Username: payload.Name,
				GoogleID: payload.Sub, Role: "user", IsVerified: true}
			if err := uc.userRepo.Create(ctx, user); err != nil {
				return nil, err
			}
			// Sinkronin profil ke user-service lewat gRPC (service terpisah, container beda).
			// Fire-and-forget: error diabaikan — auth-service tetap sumber kebenaran buat
			// login, user-service cuma "cache" profil untuk dipakai service lain.
			_, _ = uc.userClient.CreateUser(ctx, &userpb.CreateUserRequest{
				UserId: user.ID, Email: user.Email, Username: user.Username, Role: user.Role,
			})
		} else if err := uc.userRepo.LinkGoogleID(ctx, user.ID, payload.Sub); err != nil {
			// Langkah 4b: email sudah ada -> link akun Google ke user yang sudah ada, bukan bikin baru
			return nil, err
		}
	}

	// Langkah 5: issue JWT, reuse fungsi yang sama dengan login biasa
	return uc.issueTokens(user)
}

func (uc *authUsecase) issueTokens(user *domain.User) (*AuthResult, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID, "email": user.Email,
		"exp": time.Now().Add(15 * time.Minute).Unix(), // access token umur pendek (15 menit)
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(uc.jwtSecret))
	if err != nil {
		return nil, err
	}

	refreshBytes := make([]byte, 32)
	if _, err := rand.Read(refreshBytes); err != nil { // random byte, jadi refresh token tidak bisa ditebak
		return nil, err
	}
	refresh := hex.EncodeToString(refreshBytes) // ubah byte jadi string hex biar bisa dikirim di JSON
	// project ini TIDAK menyimpan refresh token ke DB — kalau kelompokmu butuh
	// rotate/revoke, tambahkan tabel refresh_tokens + repo sendiri di sini.
	return &AuthResult{AccessToken: access, RefreshToken: refresh, User: UserInfo{
		ID: user.ID, Email: user.Email, Username: user.Username, Role: user.Role,
	}}, nil
}
```
> Karena ini setup **microservice** (bukan monolith), sinkronisasi profil ke `user-service` lewat gRPC (`userpb.UserServiceClient`) adalah tambahan nyata di luar pola generik — kalau kelompokmu monolith, langkah ini dihapus saja.

GORM auto-migrate dipanggil di `cmd/main.go`: `db.AutoMigrate(&domain.User{}, &domain.OTPCode{})` — bikin/update tabel otomatis sesuai struct saat startup.

### 1.6 Endpoint (Gin)
`services/auth-service/internal/delivery/http/handler.go`:
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
Registrasi rute di `router.go`:
```go
auth := api.Group("/auth")
auth.POST("/login/google", h.LoginGoogle)
```

### 1.7 Proteksi endpoint dengan JWT
Handler juga punya `JWTMiddleware()` yang dipasang ke rute privat (mis. `GET /api/me`, dipanggil `DashboardPage.tsx` buat buktiin token valid). Validasi token **tidak** di-parse langsung di middleware — dia manggil `uc.ValidateToken(...)` di usecase layer, supaya semua logic JWT (issue & validate) tetap 1 tempat.
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

### 1.8 Kalau beda

| Kalau... | Yang berubah |
|---|---|
| Provider lain (GitHub dll) | Ganti URL verifikasi + nama field payload (mis. GitHub: `GET api.github.com/user` dengan header `Authorization: Bearer <token>`, field `id` bukan `sub`). Pola verify → cari-by-provider-id → cari-by-email → buat baru tetap sama. |
| Redirect flow, bukan popup | Frontend redirect ke `accounts.google.com/o/oauth2/v2/auth?...&response_type=code`. Backend bikin endpoint `GET /callback?code=` yang POST `code+client_id+client_secret+redirect_uri` ke `oauth2.googleapis.com/token`, dapat `id_token` balik, lanjut proses sama seperti 1.4. Di sinilah `client_secret` baru benar-benar dipakai. |
| Semua role vs role tertentu doang | Tambah/hapus 1 `if user.Role == "..." { return error }` setelah user didapat, di luar 5 langkah inti `LoginGoogle`. |
| Monolith, bukan microservice | Hapus panggilan gRPC `uc.userClient.CreateUser(...)` di langkah 4a — gak perlu sinkron ke service lain. |

---

## 2. SMTP Verification Code

### 2.1 Alur
```
generate OTP -> simpan ke DB (expires_at) -> kirim email via SMTP -> user submit -> cocokkan
```

### 2.2 Config
```
SMTP_HOST=sandbox.smtp.mailtrap.io   # atau smtp.gmail.com (butuh App Password, bukan password akun biasa)
SMTP_PORT=587                        # 587 = STARTTLS (upgrade koneksi ke terenkripsi setelah connect)
SMTP_USERNAME=xxxxx
SMTP_PASSWORD=xxxxx
SMTP_FROM=noreply@example.com        # alamat pengirim yang muncul di kolom "From" email
```

### 2.3 Kirim email
`services/auth-service/internal/mailer/smtp.go` — dibungkus struct + interface (`usecase.Mailer`) biar usecase gak bergantung ke `net/smtp` langsung:
```go
type Config struct {
	Host, Port, Username, Password, From string
}

type SMTPMailer struct{ cfg Config }

func New(cfg Config) *SMTPMailer { return &SMTPMailer{cfg: cfg} }

func (m *SMTPMailer) Send(to, subject, htmlBody string) error {
	// PlainAuth = mekanisme autentikasi SMTP dasar pakai username+password
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)

	// Raw email harus format RFC 822/MIME: header dipisah "\r\n" (BUKAN cuma "\n")
	msg := "MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" + // bilang ke email client ini HTML, bukan plain text
		"From: " + m.cfg.From + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"\r\n" + htmlBody // baris kosong = pemisah wajib antara header dan body

	addr := m.cfg.Host + ":" + m.cfg.Port
	return smtp.SendMail(addr, auth, m.cfg.From, []string{to}, []byte(msg))
	// SendMail: connect ke server, upgrade STARTTLS otomatis kalau didukung, auth, kirim, lalu close koneksi
}
```
Pakai: `mailer.New(cfg).Send(user.Email, "Kode Reset Password", otpEmailBody(code))`. Port 465 (SSL implisit dari awal koneksi, beda dari STARTTLS) butuh `tls.Dial` manual — `smtp.SendMail` bawaan cuma support STARTTLS (587/2525).

### 2.4 Generate OTP
`services/auth-service/internal/usecase/otp.go`:
```go
func generateOTP() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil { // crypto/rand, BUKAN math/rand — math/rand predictable & bisa ditebak attacker
		return "", err
	}
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2]) // gabung 3 byte jadi 1 angka (24-bit, max ~16 juta)
	return fmt.Sprintf("%06d", n%1000000), nil    // modulo 1 juta -> 6 digit; %06d pad leading zero (mis. 42 -> "000042")
}
```

### 2.5 Simpan & verifikasi (repository + usecase, GORM)

Sama seperti bagian 1.5, `OTPCode` dipisah jadi interface (`domain.OTPRepository`) + implementasi GORM (`repository/otp_repository.go`):
```go
// domain/entity.go
type OTPCode struct {
	ID        string `gorm:"primaryKey"`
	UserID    string
	Code      string
	ExpiresAt time.Time
	UsedAt    *time.Time // pointer supaya bisa NULL; nil = belum dipakai, ada isinya = sudah dipakai
	CreatedAt time.Time
}

// repository/otp_repository.go
// FindLatestUnused: ambil kode TERBARU milik user yang belum pernah dipakai.
func (r *otpRepository) FindLatestUnused(ctx context.Context, userID string) (*domain.OTPCode, error) {
	var otp domain.OTPCode
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND used_at IS NULL", userID).
		Order("created_at desc").First(&otp).Error
	if err != nil {
		return nil, err
	}
	return &otp, nil
}

// MarkUsed: tandai used_at supaya kode ini gak bisa dipakai ulang (anti-replay).
func (r *otpRepository) MarkUsed(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&domain.OTPCode{}).Where("id = ?", id).
		Update("used_at", gorm.Expr("NOW()")).Error
}
```

Usecase (`usecase/auth_usecase.go`) cari user **by email** dulu (bukan langsung by `userID`), karena endpoint publik cuma tahu email user, bukan ID internal:
```go
func (uc *authUsecase) ForgotPassword(ctx context.Context, email string) error {
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		// tetap dianggap "berhasil" di layer HTTP biar gak bocorin email mana yang terdaftar
		return nil
	}

	code, err := generateOTP()
	if err != nil {
		return err
	}
	otp := &domain.OTPCode{ID: newID(), UserID: user.ID, Code: code,
		ExpiresAt: time.Now().Add(5 * time.Minute), CreatedAt: time.Now()}
	if err := uc.otpRepo.Create(ctx, otp); err != nil {
		return err
	}
	return uc.mailer.Send(user.Email, "Kode Reset Password", otpEmailBody(code))
}

func (uc *authUsecase) VerifyOTP(ctx context.Context, email, code string) error {
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil || user == nil {
		return errors.New("invalid code")
	}
	otp, err := uc.otpRepo.FindLatestUnused(ctx, user.ID)
	if err != nil {
		return errors.New("no otp found")
	}
	if time.Now().After(otp.ExpiresAt) {
		return errors.New("otp expired")
	}
	if otp.Code != code {
		return errors.New("otp invalid")
	}
	return uc.otpRepo.MarkUsed(ctx, otp.ID)
}
```
Handler-nya (`POST /api/auth/forgot-password` dan `POST /api/auth/verify-otp`) tinggal `ShouldBindJSON` lalu panggil 2 fungsi di atas — lihat `delivery/http/handler.go`.

GORM auto-migrate (opsional, ganti bikin manual `CREATE TABLE`): `db.AutoMigrate(&User{}, &OTPCode{})` — bikin/update tabel otomatis sesuai struct saat startup, cocok buat quiz biar cepat.

### 2.6 Kalau beda

| Kalau... | Yang berubah |
|---|---|
| Service dipisah (gRPC) | `Send` di `mailer.SMTPMailer` tetap sama persis, dibungkus 1 gRPC handler yang manggil fungsi ini. Kalau monolith: panggil langsung dari usecase register/forgot-password, skip gRPC. |
| Verifikasi pakai link, bukan kode | Generate token random hex 32 byte (`crypto/rand`), kirim URL `?token=xxx` di body email, endpoint verifikasi jadi `GET` (user klik dari email) bukan `POST` body JSON. |
| Storage pakai Redis, bukan SQL | `SET otp:<user_id> code EX 300` lalu `GET`/`DEL` — TTL Redis otomatis handle expired, gak perlu cek `expires_at` manual lagi. |
| Panjang kode beda | Ganti `%06d` + `n%1000000` (mis. `%04d` + `n%10000` untuk 4 digit). |

---

## 3. Checklist
- [ ] `npm install @react-oauth/google`, bungkus `GoogleOAuthProvider`, panggil lewat `api.loginGoogle()` (bukan `fetch` inline).
- [ ] `verifyGoogleIDToken`: cek status 200, `sub`/`email` tidak kosong, `aud == client_id`.
- [ ] Urutan cari user: **by google_id → by email (link) → buat baru (IsVerified=true)**.
- [ ] User/OTP repository dipisah jadi *interface* (`domain`) + implementasi GORM (`repository`) — usecase gak boleh import `gorm.io/gorm` langsung.
- [ ] Kalau setup microservice: sinkron profil user baru ke service lain lewat gRPC (fire-and-forget), skip kalau monolith.
- [ ] SMTP: header MIME pakai `\r\n`, baris kosong pemisah header/body.
- [ ] OTP: `crypto/rand`, format `%06d`.
- [ ] `ForgotPassword`/`VerifyOTP` cari user **by email**, bukan by ID — endpoint publik cuma tahu email.
- [ ] OTP dicek expired + ditandai `used_at` setelah dipakai.
- [ ] Endpoint privat (`/api/me`) diproteksi `JWTMiddleware()`, validasi token lewat `uc.ValidateToken(...)` di usecase, bukan parse JWT langsung di middleware.
- [ ] Sebelum nulis: cek nama tabel/kolom & pola error punya kelompokmu, ganti query di atas sesuai itu.

Lihat juga [OAuth_smtp_notes.md](OAuth_smtp_notes.md) untuk versi ringkas (cuma frontend, verifikasi id_token, dan handler endpoint ditulis lengkap; sisanya dijelaskan sebagai alur).
