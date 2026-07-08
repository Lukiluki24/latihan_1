import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, clearToken, getToken } from '../lib/api'

export default function DashboardPage() {
  const navigate = useNavigate()
  const [email, setEmail] = useState<string | null>(null)

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

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12, minHeight: '100vh', alignItems: 'center', justifyContent: 'center' }}>
      <p style={{ fontSize: 20 }}>Welcome to Dashboard{email ? `, ${email}` : ''}</p>
      <button onClick={handleLogout} style={{ padding: '6px 12px' }}>Logout</button>
    </div>
  )
}
