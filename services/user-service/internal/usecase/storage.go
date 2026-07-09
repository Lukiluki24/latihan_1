package usecase

import (
	"context"
	"io"
)

// ImageStorage = interface yang dibutuhkan usecase, diimplementasikan oleh
// internal/storage (dependency inversion, sama pola dengan repository).
type ImageStorage interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, string, error)
}

const maxAvatarBytes = 2 * 1024 * 1024 // 2MB

// map content-type -> extension, sekaligus jadi whitelist tipe yang diterima.
var allowedAvatarTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}
