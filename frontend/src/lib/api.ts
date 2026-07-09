// Base path "/api" — di docker-compose ini di-proxy oleh nginx ke backend,
// jadi frontend & backend keliatan 1 origin (gak perlu mikirin CORS di production).
const BASE = '/api'

export function getToken(): string | null {
  return localStorage.getItem('access_token')
}

export function setToken(token: string) {
  localStorage.setItem('access_token', token)
}

export function clearToken() {
  localStorage.removeItem('access_token')
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(BASE + path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(getToken() ? { Authorization: `Bearer ${getToken()}` } : {}),
      ...options.headers,
    },
  })

  const data = await res.json().catch(() => null)
  if (!res.ok) {
    throw new Error(data?.error || `Request failed with status ${res.status}`)
  }
  return data as T
}

export type AuthResult = {
  access_token: string
  refresh_token: string
  user: { id: string; email: string; username: string; role: string }
}

export const api = {
  loginGoogle: (idToken: string) =>
    request<AuthResult>('/auth/login/google', {
      method: 'POST',
      body: JSON.stringify({ id_token: idToken }),
    }),

  forgotPassword: (email: string) =>
    request<{ message: string }>('/auth/forgot-password', {
      method: 'POST',
      body: JSON.stringify({ email }),
    }),

  me: () => request<{ user_id: string; email: string }>('/me'),

  // Multipart upload gak lewat request() biasa: Content-Type (dengan boundary)
  // harus di-set browser sendiri, jadi kita gak boleh maksa 'application/json'.
  // Path-nya /users/me/avatar (bukan /me/avatar) karena avatar itu data profil,
  // di-handle user-service — nginx proxy /api/users/* ke situ, beda dari
  // /api/* lain yang ke auth-service.
  uploadAvatar: async (file: File) => {
    const form = new FormData()
    form.append('avatar', file)
    const res = await fetch(BASE + '/users/me/avatar', {
      method: 'PUT',
      headers: getToken() ? { Authorization: `Bearer ${getToken()}` } : {},
      body: form,
    })
    const data = await res.json().catch(() => null)
    if (!res.ok) throw new Error(data?.error || `Request failed with status ${res.status}`)
    return data as { avatar_url: string }
  },
}
