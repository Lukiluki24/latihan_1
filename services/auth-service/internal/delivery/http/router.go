package http

import "github.com/gin-gonic/gin"

func SetupRoutes(r *gin.Engine, h *Handler) {
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		auth.POST("/login/google", h.LoginGoogle)
		auth.POST("/forgot-password", h.ForgotPassword)
		auth.POST("/verify-otp", h.VerifyOTP)

		api.GET("/me", h.JWTMiddleware(), h.Me)
	}
}

// corsMiddleware: izinkan semua origin. Cukup untuk latihan — di docker-compose
// frontend & backend sudah 1 origin lewat nginx jadi CORS gak kepake, tapi ini
// tetap berguna kalau akses backend langsung (mis. `npm run dev` port 5173).
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
