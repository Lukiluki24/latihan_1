package domain

import "time"

// User = entity inti. Tag gorm ditaruh langsung di sini (bukan di layer repository)
// supaya tetap simpel — di project besar biasanya dipisah jadi persistence model
// sendiri, tapi untuk latihan ini 1 struct dipakai bareng.
type User struct {
	ID         string `gorm:"primaryKey"`
	Email      string `gorm:"uniqueIndex"`
	Username   string
	GoogleID   string `gorm:"column:google_id;uniqueIndex"`
	Role       string
	IsVerified bool
	CreatedAt  time.Time
}

// OTPCode = kode verifikasi yang dikirim ke email (dipakai untuk forgot-password).
type OTPCode struct {
	ID        string `gorm:"primaryKey"`
	UserID    string
	Code      string
	ExpiresAt time.Time
	UsedAt    *time.Time // nil = belum dipakai
	CreatedAt time.Time
}
