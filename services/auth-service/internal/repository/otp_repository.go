package repository

import (
	"context"

	"gorm.io/gorm"

	"latihan_auth/services/auth-service/internal/domain"
)

type otpRepository struct {
	db *gorm.DB
}

func NewOTPRepository(db *gorm.DB) domain.OTPRepository {
	return &otpRepository{db: db}
}

func (r *otpRepository) Create(ctx context.Context, otp *domain.OTPCode) error {
	return r.db.WithContext(ctx).Create(otp).Error
}

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
