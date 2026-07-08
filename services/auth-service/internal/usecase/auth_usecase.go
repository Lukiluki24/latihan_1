package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	userpb "latihan_auth/generated/go/user"
	"latihan_auth/services/auth-service/internal/domain"
)

// Mailer = interface yang dibutuhkan usecase, diimplementasikan oleh internal/mailer
// (dependency inversion: usecase gak tau/gak peduli itu SMTP atau apapun).
type Mailer interface {
	Send(to, subject, htmlBody string) error
}

type UserInfo struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type AuthResult struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	User         UserInfo `json:"user"`
}

type TokenClaims struct {
	UserID string
	Email  string
}

type AuthUsecase interface {
	LoginGoogle(ctx context.Context, idToken string) (*AuthResult, error)
	ForgotPassword(ctx context.Context, email string) error
	VerifyOTP(ctx context.Context, email, code string) error
	ValidateToken(ctx context.Context, tokenStr string) (*TokenClaims, error)
}

type authUsecase struct {
	userRepo       domain.UserRepository
	otpRepo        domain.OTPRepository
	mailer         Mailer
	userClient     userpb.UserServiceClient // gRPC client ke user-service, sinkronin profil
	jwtSecret      string
	googleClientID string
}

func NewAuthUsecase(
	userRepo domain.UserRepository,
	otpRepo domain.OTPRepository,
	mailer Mailer,
	userClient userpb.UserServiceClient,
	jwtSecret string,
	googleClientID string,
) AuthUsecase {
	return &authUsecase{
		userRepo:       userRepo,
		otpRepo:        otpRepo,
		mailer:         mailer,
		userClient:     userClient,
		jwtSecret:      jwtSecret,
		googleClientID: googleClientID,
	}
}

// LoginGoogle: verifikasi id_token ke Google -> cari/link/buat user -> issue JWT.
// Urutan pencarian: by google_id -> by email (link akun) -> buat baru (verified langsung
// karena Google sudah verifikasi emailnya, gak perlu OTP lagi).
func (uc *authUsecase) LoginGoogle(ctx context.Context, idToken string) (*AuthResult, error) {
	payload, err := verifyGoogleIDToken(ctx, idToken, uc.googleClientID)
	if err != nil {
		return nil, fmt.Errorf("google token invalid: %w", err)
	}

	user, err := uc.userRepo.FindByGoogleID(ctx, payload.Sub)
	if err != nil {
		return nil, err
	}

	if user == nil {
		user, err = uc.userRepo.FindByEmail(ctx, payload.Email)
		if err != nil {
			return nil, err
		}
		if user == nil {
			user = &domain.User{
				ID:         newID(),
				Email:      payload.Email,
				Username:   payload.Name,
				GoogleID:   payload.Sub,
				Role:       "user",
				IsVerified: true,
			}
			if err := uc.userRepo.Create(ctx, user); err != nil {
				return nil, err
			}
			// Sinkronin profil ke user-service lewat gRPC. Fire-and-forget (error
			// diabaikan) sama seperti pola aslinya — auth-service tetap sumber
			// kebenaran buat login, user-service cuma "cache" profil.
			_, _ = uc.userClient.CreateUser(ctx, &userpb.CreateUserRequest{
				UserId:   user.ID,
				Email:    user.Email,
				Username: user.Username,
				Role:     user.Role,
			})
		} else if err := uc.userRepo.LinkGoogleID(ctx, user.ID, payload.Sub); err != nil {
			return nil, err
		}
	}

	return uc.issueTokens(user)
}

// ForgotPassword: generate OTP, simpan, kirim email. Ini endpoint yang dipakai
// buat testing kirim email SMTP.
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

	otp := &domain.OTPCode{
		ID:        newID(),
		UserID:    user.ID,
		Code:      code,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		CreatedAt: time.Now(),
	}
	if err := uc.otpRepo.Create(ctx, otp); err != nil {
		return err
	}

	return uc.mailer.Send(user.Email, "Kode Reset Password", otpEmailBody(code))
}

// VerifyOTP: cocokkan kode yang diinput user dengan yang tersimpan.
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

// ValidateToken: dipanggil middleware HTTP buat proteksi endpoint (mis. GET /api/me).
func (uc *authUsecase) ValidateToken(ctx context.Context, tokenStr string) (*TokenClaims, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(uc.jwtSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid or expired token")
	}

	userID, _ := claims["user_id"].(string)
	email, _ := claims["email"].(string)
	return &TokenClaims{UserID: userID, Email: email}, nil
}

func (uc *authUsecase) issueTokens(user *domain.User) (*AuthResult, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"exp":     time.Now().Add(15 * time.Minute).Unix(),
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(uc.jwtSecret))
	if err != nil {
		return nil, err
	}

	refreshBytes := make([]byte, 32)
	if _, err := rand.Read(refreshBytes); err != nil {
		return nil, err
	}
	refresh := hex.EncodeToString(refreshBytes)

	return &AuthResult{
		AccessToken:  access,
		RefreshToken: refresh,
		User: UserInfo{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
			Role:     user.Role,
		},
	}, nil
}
