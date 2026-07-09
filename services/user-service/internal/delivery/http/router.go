package http

import "github.com/gin-gonic/gin"

func SetupRoutes(r *gin.Engine, h *Handler) {
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	// Grup /api/users supaya nginx bisa bedain rute ini dari punya auth-service
	// (/api/...) sekaligus proxy ke service yang beda.
	api := r.Group("/api/users")
	{
		api.PUT("/me/avatar", h.JWTMiddleware(), h.UpdateAvatar)
		api.GET("/media/avatars/:userId/:filename", h.GetAvatar)
	}
}
