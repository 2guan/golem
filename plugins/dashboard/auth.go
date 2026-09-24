package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type userContextKey struct{}

// JWTClaims 简易轻量 JWT Claims 结构
type JWTClaims struct {
	Subject   string `json:"sub"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// HashPassword 使用 bcrypt 对密码进行安全哈希
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash 比对密码与哈希
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateRandomSecret 生成强随机密钥
func GenerateRandomSecret(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// GenerateJWT 签发 JWT
func GenerateJWT(username string, secret string, duration time.Duration) (string, int64, error) {
	if secret == "" {
		return "", 0, errors.New("jwt secret is empty")
	}

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	headerJSON, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

	now := time.Now()
	expireAt := now.Add(duration).Unix()
	claims := JWTClaims{
		Subject:   username,
		IssuedAt:  now.Unix(),
		ExpiresAt: expireAt,
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", 0, err
	}
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	signatureB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	token := signingInput + "." + signatureB64
	return token, expireAt, nil
}

// ParseAndVerifyJWT 验证并解析 JWT
func ParseAndVerifyJWT(token string, secret string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	headerB64, claimsB64, signatureB64 := parts[0], parts[1], parts[2]
	signingInput := headerB64 + "." + claimsB64

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expectedSig := mac.Sum(nil)

	actualSig, err := base64.RawURLEncoding.DecodeString(signatureB64)
	if err != nil {
		return nil, errors.New("invalid signature encoding")
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, errors.New("signature mismatch")
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(claimsB64)
	if err != nil {
		return nil, errors.New("invalid claims encoding")
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, errors.New("invalid claims json")
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return nil, errors.New("token expired")
	}

	return &claims, nil
}

// AuthMiddleware 校验请求中的 Authorization 头部或 Cookie
func (p *DashboardPlugin) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var tokenStr string
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else if cookie, err := r.Cookie("golem_token"); err == nil {
			tokenStr = cookie.Value
		} else if qToken := r.URL.Query().Get("token"); qToken != "" {
			tokenStr = qToken
		}

		if tokenStr == "" {
			http.Error(w, `{"error":"unauthorized","message":"缺少认证 Token"}`, http.StatusUnauthorized)
			return
		}

		secret := p.store.GetAdmin().JWTSecret
		claims, err := ParseAndVerifyJWT(tokenStr, secret)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"unauthorized","message":"%s"}`, err.Error()), http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey{}, claims)
		next(w, r.WithContext(ctx))
	}
}
