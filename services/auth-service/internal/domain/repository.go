package domain

import "context"

// UserRepository = kontrak layer data untuk User. Usecase cuma bergantung ke
// interface ini, bukan ke GORM langsung — implementasinya ada di internal/repository.
type UserRepository interface {
	FindByGoogleID(ctx context.Context, googleID string) (*User, error) // nil,nil kalau gak ketemu
	FindByEmail(ctx context.Context, email string) (*User, error)
	Create(ctx context.Context, u *User) error
	LinkGoogleID(ctx context.Context, userID, googleID string) error
}

// OTPRepository = kontrak layer data untuk kode verifikasi.
type OTPRepository interface {
	Create(ctx context.Context, otp *OTPCode) error
	FindLatestUnused(ctx context.Context, userID string) (*OTPCode, error)
	MarkUsed(ctx context.Context, id string) error
}
