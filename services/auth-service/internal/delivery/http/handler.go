package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"latihan_auth/services/auth-service/internal/usecase"
)

type Handler struct {
	uc usecase.AuthUsecase
}

func NewHandler(uc usecase.AuthUsecase) *Handler {
	return &Handler{uc: uc}
}

// POST /api/auth/login/google
func (h *Handler) LoginGoogle(c *gin.Context) {
	var body struct {
		IDToken string `json:"id_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id_token is required"})
		return
	}

	result, err := h.uc.LoginGoogle(c.Request.Context(), body.IDToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// POST /api/auth/forgot-password — dipakai buat testing kirim email SMTP.
func (h *Handler) ForgotPassword(c *gin.Context) {
	var body struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid email is required"})
		return
	}

	if err := h.uc.ForgotPassword(c.Request.Context(), body.Email); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// selalu balikin pesan yang sama biar gak bocorin email mana yang terdaftar
	c.JSON(http.StatusOK, gin.H{"message": "If the email exists, a reset code has been sent"})
}

// POST /api/auth/verify-otp
func (h *Handler) VerifyOTP(c *gin.Context) {
	var body struct {
		Email string `json:"email" binding:"required,email"`
		Code  string `json:"code" binding:"required,len=6"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email and 6-digit code are required"})
		return
	}

	if err := h.uc.VerifyOTP(c.Request.Context(), body.Email, body.Code); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Code verified"})
}

// GET /api/me — endpoint terproteksi JWT, dipanggil dashboard buat buktiin token valid.
func (h *Handler) Me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"user_id": c.GetString("user_id"),
		"email":   c.GetString("email"),
	})
}

// JWTMiddleware: proteksi endpoint, wajib header "Authorization: Bearer <token>".
// Validasi lewat usecase (bukan parse JWT langsung di sini) supaya logic token
// tetap 1 tempat di usecase layer.
func (h *Handler) JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		claims, err := h.uc.ValidateToken(c.Request.Context(), strings.TrimPrefix(raw, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Next()
	}
}
