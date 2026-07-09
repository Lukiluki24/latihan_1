package http

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"latihan_auth/services/user-service/internal/usecase"
)

type Handler struct {
	uc        usecase.UserUsecase
	jwtSecret string
}

func NewHandler(uc usecase.UserUsecase, jwtSecret string) *Handler {
	return &Handler{uc: uc, jwtSecret: jwtSecret}
}

// PUT /api/users/me/avatar — endpoint terproteksi, upload avatar lewat multipart form (field "avatar").
func (h *Handler) UpdateAvatar(c *gin.Context) {
	file, _, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "avatar file is required"})
		return
	}
	defer file.Close()

	// Limit dibatasi lebih longgar dari maxAvatarBytes di usecase — biar validasi
	// "final" tetap 1 tempat di usecase, ini cuma jaga-jaga baca stream gak kebablasan.
	data, err := io.ReadAll(io.LimitReader(file, 4*1024*1024))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	key, err := h.uc.UpdateAvatar(c.Request.Context(), c.GetString("user_id"), data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"avatar_url": "/api/users/media/" + key})
}

// GET /api/users/media/avatars/:userId/:filename — publik (gak pakai JWTMiddleware),
// karena avatar memang dimaksudkan buat ditampilkan di UI manapun.
func (h *Handler) GetAvatar(c *gin.Context) {
	body, contentType, err := h.uc.GetAvatar(c.Request.Context(), c.Param("userId"), c.Param("filename"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "avatar not found"})
		return
	}
	defer body.Close()
	// nosniff: cegah browser nebak-nebak ulang tipe konten sendiri (mitigasi
	// stored-content confusion kalau ternyata ada file lolos validasi tapi isinya beda).
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, -1, contentType, body, nil)
}

// JWTMiddleware: user-service verifikasi token JWT SENDIRI (stateless), gak
// manggil balik ke auth-service tiap request. Bisa gitu karena secret & bentuk
// claim ("user_id", "email") sama persis dengan yang di-issue auth-service
// (lihat auth-service/internal/usecase/auth_usecase.go issueTokens) — makanya
// JWT_SECRET wajib sama di kedua service.
func (h *Handler) JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		claims := jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(strings.TrimPrefix(raw, "Bearer "), claims, func(t *jwt.Token) (interface{}, error) {
			return []byte(h.jwtSecret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		userID, _ := claims["user_id"].(string)
		c.Set("user_id", userID)
		c.Next()
	}
}
