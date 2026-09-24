package main

import (
	"testing"
	"time"
)

func TestPasswordHash(t *testing.T) {
	password := "SecretPass123!"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !CheckPasswordHash(password, hash) {
		t.Errorf("CheckPasswordHash failed for correct password")
	}

	if CheckPasswordHash("WrongPass", hash) {
		t.Errorf("CheckPasswordHash succeeded for wrong password")
	}
}

func TestJWTGenerationAndVerification(t *testing.T) {
	secret := GenerateRandomSecret(32)
	username := "admin"

	token, expireAt, err := GenerateJWT(username, secret, 1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}
	if expireAt <= time.Now().Unix() {
		t.Errorf("expireAt is not in the future")
	}

	claims, err := ParseAndVerifyJWT(token, secret)
	if err != nil {
		t.Fatalf("ParseAndVerifyJWT failed: %v", err)
	}
	if claims.Subject != username {
		t.Errorf("expected subject %s, got %s", username, claims.Subject)
	}

	// 篡改签名验证失败
	tamperedToken := token + "tamper"
	if _, err := ParseAndVerifyJWT(tamperedToken, secret); err == nil {
		t.Errorf("expected error for tampered token, got nil")
	}

	// 错误密钥验证失败
	if _, err := ParseAndVerifyJWT(token, "wrong_secret"); err == nil {
		t.Errorf("expected error for wrong secret, got nil")
	}
}
