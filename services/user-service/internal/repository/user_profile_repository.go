package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"latihan_auth/services/user-service/internal/domain"
)

type userProfileRepository struct {
	db *gorm.DB
}

func NewUserProfileRepository(db *gorm.DB) domain.UserProfileRepository {
	return &userProfileRepository{db: db}
}

func (r *userProfileRepository) Create(ctx context.Context, p *domain.UserProfile) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *userProfileRepository) FindByID(ctx context.Context, userID string) (*domain.UserProfile, error) {
	var p domain.UserProfile
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}
