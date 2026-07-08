import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { GoogleLogin, GoogleOAuthProvider } from '@react-oauth/google'
import { api, setToken } from '../lib/api'

const GOOGLE_CLIENT_ID = import.meta.env.VITE_GOOGLE_CLIENT_ID as string

export default function LoginPage() {
  const navigate = useNavigate()
  const [status, setStatus] = useState<string>('')
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)

  // Dipanggil setelah Google berhasil kasih id_token ke browser.
  async function handleGoogleSuccess(idToken: string) {
    setStatus('')
    try {
      const result = await api.loginGoogle(idToken)
      setToken(result.access_token) // simpan JWT, dipakai buat request ke /api/me di Dashboard
      navigate('/dashboard')
    } catch (err) {
      setStatus(err instanceof Error ? err.message : 'Google login failed')
    }
  }

  // Cuma buat testing kirim email SMTP — kirim kode reset ke email yang diinput.
  async function handleForgotPassword(e: React.FormEvent) {
    e.preventDefault()
    if (!email) return
    setLoading(true)
    setStatus('')
    try {
      const result = await api.forgotPassword(email)
      setStatus(result.message)
    } catch (err) {
      setStatus(err instanceof Error ? err.message : 'Failed to send email')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ display: 'flex', minHeight: '100vh', alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ width: 320, display: 'flex', flexDirection: 'column', gap: 24 }}>
        <h1 style={{ textAlign: 'center', fontSize: 24 }}>Latihan Auth</h1>

        <div style={{ display: 'flex', justifyContent: 'center' }}>
          <GoogleOAuthProvider clientId={GOOGLE_CLIENT_ID}>
            <GoogleLogin
              onSuccess={(res) => res.credential && handleGoogleSuccess(res.credential)}
              onError={() => setStatus('Google login failed')}
            />
          </GoogleOAuthProvider>
        </div>

        <hr />

        <form onSubmit={handleForgotPassword} style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <label htmlFor="email" style={{ fontSize: 14 }}>Forgot password (test kirim email)</label>
          <input
            id="email"
            type="email"
            placeholder="you@example.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            style={{ padding: 8 }}
          />
          <button type="submit" disabled={loading} style={{ padding: 8 }}>
            {loading ? 'Sending...' : 'Send reset code'}
          </button>
        </form>

        {status && <p style={{ fontSize: 13, textAlign: 'center' }}>{status}</p>}
      </div>
    </div>
  )
}
