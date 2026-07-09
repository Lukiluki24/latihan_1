import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, clearToken, getToken } from '../lib/api'

export default function DashboardPage() {
  const navigate = useNavigate()
  const [email, setEmail] = useState<string | null>(null)
  const [avatarUrl, setAvatarUrl] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)
  const [avatarError, setAvatarError] = useState('')

  useEffect(() => {
    if (!getToken()) {
      navigate('/', { replace: true })
      return
    }
    // Buktiin JWT valid: minta data user ke endpoint yang diproteksi middleware.
    api.me()
      .then((res) => setEmail(res.email))
      .catch(() => {
        clearToken()
        navigate('/', { replace: true })
      })
  }, [navigate])

  function handleLogout() {
    clearToken()
    navigate('/', { replace: true })
  }

  // Upload avatar ke SeaweedFS lewat PUT /api/users/me/avatar (user-service),
  // lalu langsung tampilin hasilnya (gak perlu refresh/re-fetch /api/me).
  async function handleAvatarChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    setUploading(true)
    setAvatarError('')
    try {
      const result = await api.uploadAvatar(file)
      setAvatarUrl(result.avatar_url)
    } catch (err) {
      setAvatarError(err instanceof Error ? err.message : 'Upload failed')
    } finally {
      setUploading(false)
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12, minHeight: '100vh', alignItems: 'center', justifyContent: 'center' }}>
      <img
        src={avatarUrl ?? undefined}
        alt="avatar"
        style={{ width: 80, height: 80, borderRadius: '50%', objectFit: 'cover', background: '#eee' }}
      />
      <input type="file" accept="image/png,image/jpeg,image/webp" disabled={uploading} onChange={handleAvatarChange} />
      {avatarError && <p style={{ fontSize: 13, color: 'crimson' }}>{avatarError}</p>}

      <p style={{ fontSize: 20 }}>Welcome to Dashboard{email ? `, ${email}` : ''}</p>
      <button onClick={handleLogout} style={{ padding: '6px 12px' }}>Logout</button>
    </div>
  )
}
