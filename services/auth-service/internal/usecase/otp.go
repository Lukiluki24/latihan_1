package usecase

import (
	"crypto/rand"
	"fmt"

	"github.com/google/uuid"
)

// generateOTP: kode 6 digit pakai crypto/rand (bukan math/rand, biar tidak bisa ditebak).
func generateOTP() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%06d", n%1000000), nil // %06d -> selalu 6 digit, pad leading zero
}

func newID() string {
	return uuid.NewString()
}

func otpEmailBody(code string) string {
	return fmt.Sprintf(`
		<div style="font-family:Arial,sans-serif;max-width:480px;margin:0 auto;padding:32px;border:1px solid #eee;border-radius:8px;">
			<h2 style="margin:0 0 8px;">Reset Password</h2>
			<p style="color:#555;margin:0 0 24px;">Gunakan kode berikut untuk reset password kamu.</p>
			<div style="background:#f0f0f0;border-radius:6px;padding:20px;text-align:center;
				font-size:32px;font-weight:bold;letter-spacing:8px;">%s</div>
			<p style="color:#999;font-size:13px;margin:24px 0 0;">Kode berlaku 5 menit.</p>
		</div>`, code)
}
