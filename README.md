# Cheatsheet: Google OAuth 2.0 + SMTP Verification Code

Stack wajib: Go semua service, Clean Architecture, React+Vite frontend. Framework HTTP/pola repo bebas per kelompok — kode di bawah pakai **GORM** + Gin sebagai contoh konkret; tinggal ganti nama tabel/kolom kalau schema kelompokmu beda, strukturnya tetap sama.

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
`client_secret` **tidak dipakai** di pola ini (popup/One Tap) karena tidak ada tahap tukar `code` di server. Baru kepake kalau pakai redirect flow (lihat 1.7).

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

```tsx
import { GoogleLogin, GoogleOAuthProvider } from '@react-oauth/google'

// Dipanggil setelah user berhasil klik & pilih akun Google
async function handleGoogleSuccess(idToken: string) {
  const res = await fetch('/api/auth/login/google', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id_token: idToken }), // kirim id_token mentah, backend yang verifikasi
  })
  const data = await res.json()
  // simpan data.access_token / refresh_token ke store kamu (Zustand/Context/dsb)
}

<GoogleOAuthProvider clientId={import.meta.env.VITE_GOOGLE_CLIENT_ID}>
  {/* Provider inject context GIS ke child-nya, wajib membungkus <GoogleLogin> */}
  <GoogleLogin
    onSuccess={(res) => res.credential && handleGoogleSuccess(res.credential)}
    // res.credential = ID Token (JWT) hasil sign-in Google, BUKAN access_token OAuth biasa
  />
</GoogleOAuthProvider>
```
`.env`: `VITE_GOOGLE_CLIENT_ID=xxxx.apps.googleusercontent.com` — prefix `VITE_` wajib, tanpa itu Vite tidak akan expose variabel ke bundle browser.

### 1.4 Backend — verifikasi id_token
```go
// Field JSON tag harus PERSIS sama nama field yang dibalikin Google, kalau salah ketik hasilnya kosong bukan error
type GoogleIDTokenPayload struct {
	Sub   string `json:"sub"`   // Google User ID unik permanen — pakai ini sebagai identifier, JANGAN email (email bisa ganti)
	Email string `json:"email"`
	Name  string `json:"name"`
	Aud   string `json:"aud"`   // audience = client_id tujuan token ini dibuat, wajib dicek biar token dari app Google lain ditolak
}

func VerifyGoogleIDToken(ctx context.Context, idToken, clientID string) (*GoogleIDTokenPayload, error) {
	// GET ke endpoint publik Google, id_token dikirim sebagai query param
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://oauth2.googleapis.com/tokeninfo?id_token="+idToken, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // wajib ditutup, kalau lupa -> connection leak

	if resp.StatusCode != http.StatusOK {
		// Google balikin non-200 kalau token invalid/sudah expired/salah format
		return nil, fmt.Errorf("invalid google token")
	}

	var p GoogleIDTokenPayload
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
Tidak butuh library eksternal, cukup `net/http` + `encoding/json` bawaan Go — cara paling cepat ditulis saat quiz.

### 1.5 Data layer + usecase (konkret, GORM)

```go
type User struct {
	ID       string `gorm:"primaryKey"`
	Email    string
	Username string
	GoogleID string `gorm:"column:google_id"` // nullable secara alami di DB (string kosong "" kalau belum link)
	Role     string
	Verified bool   `gorm:"column:is_verified"`
}

// Cari user yang SUDAH PERNAH login Google sebelumnya (google_id sudah ke-link)
func findUserByGoogleID(ctx context.Context, db *gorm.DB, googleID string) (*User, error) {
	var u User
	err := db.WithContext(ctx).Where("google_id = ?", googleID).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil // gak ketemu -> nil,nil (bukan error) supaya caller gampang cek `user == nil`
	}
	return &u, err
}

// Cari user berdasarkan email — dipakai buat cek apakah user sudah daftar manual sebelumnya
func findUserByEmail(ctx context.Context, db *gorm.DB, email string) (*User, error) {
	var u User
	err := db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &u, err
}

// Bikin user baru untuk yang belum pernah terdaftar sama sekali
func createUser(ctx context.Context, db *gorm.DB, u *User) error {
	u.ID = uuid.NewString() // generate ID di sisi Go, bukan andalkan auto-increment DB
	return db.WithContext(ctx).Create(u).Error
}

// "Nempelin" google_id ke user yang sudah ada (kasus: daftar manual dulu, baru nyoba Google login)
func linkGoogleID(ctx context.Context, db *gorm.DB, userID, googleID string) error {
	return db.WithContext(ctx).Model(&User{}).Where("id = ?", userID).Update("google_id", googleID).Error
}
```

```go
type AuthResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	User         User   `json:"user"`
}

// Fungsi yang sama dipakai baik untuk login Google MAUPUN login email/password biasa
func issueTokens(user *User, jwtSecret string) (AuthResult, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID, "email": user.Email,
		"exp": time.Now().Add(15 * time.Minute).Unix(), // access token umur pendek (15 menit)
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(jwtSecret))
	if err != nil {
		return AuthResult{}, err
	}

	refreshBytes := make([]byte, 32)
	rand.Read(refreshBytes)                     // random byte, jadi refresh token tidak bisa ditebak
	refresh := hex.EncodeToString(refreshBytes)  // ubah byte jadi string hex biar bisa dikirim di JSON
	// simpan hash(refresh) + expiry ke tabel refresh_tokens di sini, sesuai punya kelompokmu (skip di contoh ini)
	return AuthResult{AccessToken: access, RefreshToken: refresh, User: *user}, nil
}

func LoginWithGoogle(ctx context.Context, db *gorm.DB, idToken, jwtSecret, googleClientID string) (AuthResult, error) {
	// Langkah 1: pastikan id_token ini valid & memang dari Google
	payload, err := VerifyGoogleIDToken(ctx, idToken, googleClientID)
	if err != nil {
		return AuthResult{}, err
	}

	// Langkah 2: sudah pernah login Google sebelumnya?
	user, err := findUserByGoogleID(ctx, db, payload.Sub)
	if err != nil {
		return AuthResult{}, err
	}

	if user == nil {
		// Langkah 3: belum pernah — cek apakah emailnya sudah terdaftar (daftar manual)
		user, err = findUserByEmail(ctx, db, payload.Email)
		if err != nil {
			return AuthResult{}, err
		}
		if user == nil {
			// Langkah 4a: user benar-benar baru -> auto-register, langsung verified
			// (Verified: true karena Google sudah memverifikasi emailnya, gak perlu OTP lagi)
			user = &User{Email: payload.Email, Username: payload.Name, GoogleID: payload.Sub, Role: "user", Verified: true}
			if err := createUser(ctx, db, user); err != nil {
				return AuthResult{}, err
			}
		} else if err := linkGoogleID(ctx, db, user.ID, payload.Sub); err != nil {
			// Langkah 4b: email sudah ada -> link akun Google ke user yang sudah ada, bukan bikin baru
			return AuthResult{}, err
		}
	}

	// Langkah 5: issue JWT, reuse fungsi yang sama dengan login biasa
	return issueTokens(user, jwtSecret)
}
```

### 1.6 Endpoint (Gin)
```go
r.POST("/api/auth/login/google", func(c *gin.Context) {
	var body struct{ IDToken string `json:"id_token" binding:"required"` }
	if err := c.ShouldBindJSON(&body); err != nil {
		// binding:"required" -> Gin otomatis reject kalau field kosong/gak ada
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	result, err := LoginWithGoogle(c.Request.Context(), db, body.IDToken, jwtSecret, googleClientID)
	if err != nil {
		c.JSON(401, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, result)
})
```

### 1.7 Kalau beda

| Kalau... | Yang berubah |
|---|---|
| Provider lain (GitHub dll) | Ganti URL verifikasi + nama field payload (mis. GitHub: `GET api.github.com/user` dengan header `Authorization: Bearer <token>`, field `id` bukan `sub`). Pola verify → cari-by-provider-id → cari-by-email → buat baru tetap sama. |
| Redirect flow, bukan popup | Frontend redirect ke `accounts.google.com/o/oauth2/v2/auth?...&response_type=code`. Backend bikin endpoint `GET /callback?code=` yang POST `code+client_id+client_secret+redirect_uri` ke `oauth2.googleapis.com/token`, dapat `id_token` balik, lanjut proses sama seperti 1.4. Di sinilah `client_secret` baru benar-benar dipakai. |
| Semua role vs role tertentu doang | Tambah/hapus 1 `if user.Role == "..." { return error }` setelah user didapat, di luar 5 langkah inti `LoginWithGoogle`. |

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
```go
func SendEmail(host, port, username, password, from, to, subject, htmlBody string) error {
	// PlainAuth = mekanisme autentikasi SMTP dasar pakai username+password
	auth := smtp.PlainAuth("", username, password, host)

	// Raw email harus format RFC 822/MIME: header dipisah "\r\n" (BUKAN cuma "\n")
	msg := "MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" + // bilang ke email client ini HTML, bukan plain text
		"From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"\r\n" + htmlBody // baris kosong = pemisah wajib antara header dan body

	return smtp.SendMail(host+":"+port, auth, from, []string{to}, []byte(msg))
	// SendMail: connect ke server, upgrade STARTTLS otomatis kalau didukung, auth, kirim, lalu close koneksi
}
```
Pakai: `SendEmail(host, port, user, pass, from, to, "Kode Verifikasi", fmt.Sprintf("<h2>%s</h2>", code))`. Port 465 (SSL implisit dari awal koneksi, beda dari STARTTLS) butuh `tls.Dial` manual — `smtp.SendMail` bawaan cuma support STARTTLS (587/2525).

### 2.4 Generate OTP
```go
func GenerateOTP() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil { // crypto/rand, BUKAN math/rand — math/rand predictable & bisa ditebak attacker
		return "", err
	}
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2]) // gabung 3 byte jadi 1 angka (24-bit, max ~16 juta)
	return fmt.Sprintf("%06d", n%1000000), nil    // modulo 1 juta -> 6 digit; %06d pad leading zero (mis. 42 -> "000042")
}
```

### 2.5 Simpan & verifikasi (konkret, GORM)
```go
type OTPCode struct {
	ID        string `gorm:"primaryKey"`
	UserID    string
	Code      string
	ExpiresAt time.Time
	UsedAt    *time.Time // pointer supaya bisa NULL; nil = belum dipakai, ada isinya = sudah dipakai
}

func saveOTP(ctx context.Context, db *gorm.DB, userID, code string) error {
	otp := OTPCode{
		ID:        uuid.NewString(),
		UserID:    userID,
		Code:      code,
		ExpiresAt: time.Now().Add(5 * time.Minute), // TTL pendek karena ini kode sekali pakai, bukan token biasa
	}
	return db.WithContext(ctx).Create(&otp).Error
}

func verifyOTP(ctx context.Context, db *gorm.DB, userID, inputCode string) error {
	var otp OTPCode
	// ambil kode TERBARU milik user ini yang belum pernah dipakai (used_at IS NULL)
	err := db.WithContext(ctx).
		Where("user_id = ? AND used_at IS NULL", userID).
		Order("id desc").First(&otp).Error
	if err != nil {
		return fmt.Errorf("no otp found")
	}
	if time.Now().After(otp.ExpiresAt) {
		return fmt.Errorf("otp expired")
	}
	if otp.Code != inputCode {
		return fmt.Errorf("otp invalid")
	}
	now := time.Now()
	// tandai used_at supaya kode ini tidak bisa dipakai lagi (mencegah replay)
	return db.WithContext(ctx).Model(&otp).Update("used_at", now).Error
}
```
GORM auto-migrate (opsional, ganti bikin manual `CREATE TABLE`): `db.AutoMigrate(&User{}, &OTPCode{})` — bikin/update tabel otomatis sesuai struct saat startup, cocok buat quiz biar cepat.

### 2.6 Kalau beda

| Kalau... | Yang berubah |
|---|---|
| Service dipisah (gRPC) | `SendEmail` tetap sama persis, dibungkus 1 gRPC handler yang manggil fungsi ini. Kalau monolith: panggil langsung dari usecase register/forgot-password, skip gRPC. |
| Verifikasi pakai link, bukan kode | Generate token random hex 32 byte (`crypto/rand`), kirim URL `?token=xxx` di body email, endpoint verifikasi jadi `GET` (user klik dari email) bukan `POST` body JSON. |
| Storage pakai Redis, bukan SQL | `SET otp:<user_id> code EX 300` lalu `GET`/`DEL` — TTL Redis otomatis handle expired, gak perlu cek `expires_at` manual lagi. |
| Panjang kode beda | Ganti `%06d` + `n%1000000` (mis. `%04d` + `n%10000` untuk 4 digit). |

---

## 3. Checklist
- [ ] `npm install @react-oauth/google`, bungkus `GoogleOAuthProvider`.
- [ ] `VerifyGoogleIDToken`: cek status 200, `sub`/`email` tidak kosong, `aud == client_id`.
- [ ] Urutan cari user: **by google_id → by email (link) → buat baru (verified=true)**.
- [ ] SMTP: header MIME pakai `\r\n`, baris kosong pemisah header/body.
- [ ] OTP: `crypto/rand`, format `%06d`.
- [ ] OTP dicek expired + ditandai `used_at` setelah dipakai.
- [ ] Sebelum nulis: cek nama tabel/kolom & pola error punya kelompokmu, ganti query di atas sesuai itu.
