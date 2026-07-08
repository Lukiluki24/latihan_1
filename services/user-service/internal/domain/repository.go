package domain

import "context"

type UserProfileRepository interface {
	Create(ctx context.Context, p *UserProfile) error
	FindByID(ctx context.Context, userID string) (*UserProfile, error) // nil,nil kalau gak ketemu
}
