package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"latihan_auth/services/user-service/internal/domain"
)

type UserUsecase interface {
	CreateUser(ctx context.Context, userID, email, username, role string) error
	GetUser(ctx context.Context, userID string) (*domain.UserProfile, error)
	UpdateAvatar(ctx context.Context, userID string, data []byte) (string, error)
	GetAvatar(ctx context.Context, userID, filename string) (io.ReadCloser, string, error)
}

type userUsecase struct {
	repo    domain.UserProfileRepository
	storage ImageStorage
}

func NewUserUsecase(repo domain.UserProfileRepository, storage ImageStorage) UserUsecase {
	return &userUsecase{repo: repo, storage: storage}
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

// UpdateAvatar: validasi tipe & ukuran, upload ke blob storage, lalu simpan
// key-nya ke kolom avatar_url profil. Key deterministik (avatars/<id>/avatar.<ext>)
// jadi upload ulang otomatis menimpa avatar lama.
//
// Tipe file di-deteksi dari ISI byte-nya (http.DetectContentType), BUKAN dari
// header Content-Type yang diklaim client — client bisa bohong (mis. upload
// file .html tapi ngaku "image/jpeg" biar lolos whitelist).
func (uc *userUsecase) UpdateAvatar(ctx context.Context, userID string, data []byte) (string, error) {
	if len(data) > maxAvatarBytes {
		return "", errors.New("file too large (max 2MB)")
	}

	sniffed := http.DetectContentType(data)
	ext, ok := allowedAvatarTypes[sniffed]
	if !ok {
		return "", errors.New("only jpg/png/webp allowed")
	}

	key := fmt.Sprintf("avatars/%s/avatar.%s", userID, ext)
	if err := uc.storage.Upload(ctx, key, data, sniffed); err != nil {
		return "", err
	}
	if err := uc.repo.UpdateAvatarURL(ctx, userID, key); err != nil {
		return "", err
	}
	return key, nil
}

// GetAvatar: dipanggil handler publik (GET /api/users/media/avatars/:userId/:filename)
// buat streaming gambar balik dari blob storage.
func (uc *userUsecase) GetAvatar(ctx context.Context, userID, filename string) (io.ReadCloser, string, error) {
	key := fmt.Sprintf("avatars/%s/%s", userID, filename)
	return uc.storage.Get(ctx, key)
}
