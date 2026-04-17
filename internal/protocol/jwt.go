package protocol

import (
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ValidateJWT validates the JWT token against the PSK, checks expiration and jti via bloom filter.
func ValidateJWT(tokenStr string, psk []byte, maxAge time.Duration, checkJTI func(string) bool) (bool, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return psk, nil
	}, jwt.WithValidMethods([]string{"HS256"}))

	if err != nil || !token.Valid {
		return false, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return false, fmt.Errorf("invalid claims format")
	}

	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return false, fmt.Errorf("missing exp claim")
	}
	if time.Now().After(exp.Time) {
		return false, fmt.Errorf("token expired")
	}

	iat, err := claims.GetIssuedAt()
	if err != nil || iat == nil {
		return false, fmt.Errorf("missing iat claim")
	}
	if time.Since(iat.Time) > maxAge {
		return false, fmt.Errorf("token issued too long ago")
	}

	jtiStr, ok := claims["jti"].(string)
	if !ok || jtiStr == "" {
		return false, fmt.Errorf("missing jti claim")
	}
	jti := jtiStr

	if checkJTI != nil && !checkJTI(jti) {
		return false, fmt.Errorf("token replayed (jti already seen)")
	}

	return true, nil
}

// GenerateJWT creates a new JWT token using the PSK.
func GenerateJWT(psk []byte, jti string, sub string, iat time.Time, exp time.Time) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iat": jwt.NewNumericDate(iat),
		"exp": jwt.NewNumericDate(exp),
		"jti": jti,
		"sub": sub,
	})

	return token.SignedString(psk)
}

// TimingSafeCompare is a wrapper for crypto/subtle.
func TimingSafeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
