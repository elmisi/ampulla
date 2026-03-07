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
	"sync"
	"time"
)

const cookieName = "ampulla_session"
const sessionDuration = 24 * time.Hour

const (
	loginRateLimit  = 5
	loginRateWindow = 5 * time.Minute
)

type Auth struct {
	username      string
	password      string
	sessionSecret []byte

	mu           sync.Mutex
	loginAttempts map[string][]time.Time
}

func NewAuth(username, password string, secret []byte) *Auth {
	return &Auth{
		username:      username,
		password:      password,
		sessionSecret: secret,
		loginAttempts: make(map[string][]time.Time),
	}
}

func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	if !a.allowLogin(ip) {
		http.Error(w, `{"error":"too many attempts"}`, http.StatusTooManyRequests)
		return
	}

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
		a.recordAttempt(ip)
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
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"user":"` + req.Username + `"}`))
}

func (a *Auth) allowLogin(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-loginRateWindow)

	attempts := a.loginAttempts[ip]
	// Remove expired entries
	valid := attempts[:0]
	for _, t := range attempts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	a.loginAttempts[ip] = valid

	return len(valid) < loginRateLimit
}

func (a *Auth) recordAttempt(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.loginAttempts[ip] = append(a.loginAttempts[ip], time.Now())
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
