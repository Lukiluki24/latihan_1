package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userpb "latihan_auth/generated/go/user"
	"latihan_auth/services/user-service/internal/usecase"
)

type UserGRPCServer struct {
	userpb.UnimplementedUserServiceServer
	uc usecase.UserUsecase
}

func NewUserGRPCServer(uc usecase.UserUsecase) *UserGRPCServer {
	return &UserGRPCServer{uc: uc}
}

func (s *UserGRPCServer) CreateUser(ctx context.Context, req *userpb.CreateUserRequest) (*userpb.CreateUserResponse, error) {
	if req.UserId == "" || req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and email are required")
	}
	if err := s.uc.CreateUser(ctx, req.UserId, req.Email, req.Username, req.Role); err != nil {
		return nil, status.Errorf(codes.Internal, "create user: %v", err)
	}
	return &userpb.CreateUserResponse{Success: true}, nil
}

func (s *UserGRPCServer) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {
	p, err := s.uc.GetUser(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "get user: %v", err)
	}
	return &userpb.GetUserResponse{
		UserId:   p.UserID,
		Email:    p.Email,
		Username: p.Username,
		Role:     p.Role,
	}, nil
}
