package domain

// UserProfile = data profil yang dipegang user-service sendiri, terpisah dari
// tabel `users` milik auth-service (yang isinya kredensial login). Disinkronin
// lewat gRPC CreateUser tiap kali ada user baru di auth-service.
type UserProfile struct {
	UserID    string `gorm:"column:user_id;primaryKey"`
	Email     string
	Username  string
	Role      string
	AvatarURL string // key di blob storage, mis. "avatars/<id>/avatar.jpg" — bukan URL penuh
}
