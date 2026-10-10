package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const TokenTTL = 24 * time.Hour

var (
	ErrTokenInvalid = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

type TokenClaims struct {
	Sub  int64  `json:"sub"`
	Name string `json:"name"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

type TokenService struct {
	secret []byte
}

func NewTokenService(secret []byte) *TokenService {
	return &TokenService{secret: secret}
}

func (t *TokenService) Generate(user AuthUser) (string, error) {
	now := time.Now().Unix()
	claims := TokenClaims{
		Sub:  user.ID,
		Name: user.Username,
		Iat:  now,
		Exp:  now + int64(TokenTTL.Seconds()),
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	enc := base64.RawURLEncoding
	payloadB64 := enc.EncodeToString(payload)

	mac := hmac.New(sha256.New, t.secret)
	mac.Write(payload)
	sig := enc.EncodeToString(mac.Sum(nil))

	return payloadB64 + "." + sig, nil
}

func (t *TokenService) Validate(token string) (TokenClaims, error) {
	dot := -1
	for i := range token {
		if token[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return TokenClaims{}, ErrTokenInvalid
	}

	payloadB64 := token[:dot]
	sigB64 := token[dot+1:]

	enc := base64.RawURLEncoding

	payload, err := enc.DecodeString(payloadB64)
	if err != nil {
		return TokenClaims{}, ErrTokenInvalid
	}

	sig, err := enc.DecodeString(sigB64)
	if err != nil {
		return TokenClaims{}, ErrTokenInvalid
	}

	mac := hmac.New(sha256.New, t.secret)
	mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return TokenClaims{}, ErrTokenInvalid
	}

	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return TokenClaims{}, ErrTokenInvalid
	}

	if time.Now().Unix() > claims.Exp {
		return TokenClaims{}, ErrTokenExpired
	}

	return claims, nil
}
