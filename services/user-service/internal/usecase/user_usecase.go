package usecase

import (
	"context"
	"errors"

	"latihan_auth/services/user-service/internal/domain"
)

type UserUsecase interface {
	CreateUser(ctx context.Context, userID, email, username, role string) error
	GetUser(ctx context.Context, userID string) (*domain.UserProfile, error)
}

type userUsecase struct {
	repo domain.UserProfileRepository
}

func NewUserUsecase(repo domain.UserProfileRepository) UserUsecase {
	return &userUsecase{repo: repo}
}

// CreateUser: idempotent secara sederhana — kalau profil sudah ada (dipanggil
// ulang, mis. auth-service retry), gak dianggap error.
func (uc *userUsecase) CreateUser(ctx context.Context, userID, email, username, role string) error {
	existing, err := uc.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	return uc.repo.Create(ctx, &domain.UserProfile{
		UserID:   userID,
		Email:    email,
		Username: username,
		Role:     role,
	})
}

func (uc *userUsecase) GetUser(ctx context.Context, userID string) (*domain.UserProfile, error) {
	p, err := uc.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("user not found")
	}
	return p, nil
}
