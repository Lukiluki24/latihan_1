package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Field JSON tag harus persis sama dengan yang dibalikin Google.
type googleIDTokenPayload struct {
	Sub   string `json:"sub"`   // Google User ID unik permanen, dipakai sebagai identifier utama
	Email string `json:"email"`
	Name  string `json:"name"`
	Aud   string `json:"aud"`   // audience = client_id tujuan token ini, wajib dicocokkan
}

// verifyGoogleIDToken: verifikasi id_token dari frontend dengan hit endpoint publik
// Google, tidak butuh library tambahan.
func verifyGoogleIDToken(ctx context.Context, idToken, clientID string) (*googleIDTokenPayload, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://oauth2.googleapis.com/tokeninfo?id_token="+idToken, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google tokeninfo status %d", resp.StatusCode)
	}

	var p googleIDTokenPayload
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	if p.Sub == "" || p.Email == "" || p.Aud != clientID {
		return nil, fmt.Errorf("invalid google payload")
	}
	return &p, nil
}
