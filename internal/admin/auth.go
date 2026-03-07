package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const cookieName = "ampulla_session"
const sessionDuration = 24 * time.Hour

type Auth struct {
	username      string
	password      string
	sessionSecret []byte
}

func NewAuth(username, password string, secret []byte) *Auth {
	return &Auth{
		username:      username,
		password:      password,
		sessionSecret: secret,
	}
}

func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if subtle.ConstantTimeCompare([]byte(req.Username), []byte(a.username)) != 1 ||
		subtle.ConstantTimeCompare([]byte(req.Password), []byte(a.password)) != 1 {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	expiry := time.Now().Add(sessionDuration).Unix()
	payload := fmt.Sprintf("%s|%d", req.Username, expiry)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	sig := a.sign(encoded)
	cookieValue := encoded + "." + sig

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    cookieValue,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"user":"` + req.Username + `"}`))
}

func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func (a *Auth) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"user":"` + a.username + `"}`))
}

func (a *Auth) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(cookie.Value, ".", 2)
		if len(parts) != 2 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		encoded, sig := parts[0], parts[1]
		if !a.verify(encoded, sig) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		decoded, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		payloadParts := strings.SplitN(string(decoded), "|", 2)
		if len(payloadParts) != 2 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		expiry, err := strconv.ParseInt(payloadParts[1], 10, 64)
		if err != nil || time.Now().Unix() > expiry {
			http.Error(w, `{"error":"session expired"}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Auth) sign(data string) string {
	h := hmac.New(sha256.New, a.sessionSecret)
	h.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func (a *Auth) verify(data, sig string) bool {
	expected := a.sign(data)
	return hmac.Equal([]byte(expected), []byte(sig))
}
