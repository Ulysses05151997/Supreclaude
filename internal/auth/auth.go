// Package auth handles password hashing, session token generation, and the
// request middleware that gates the application behind a login.
package auth

import (
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/bcrypt"
)

// dummyHash is compared against when a username is not found, so login timing
// does not reveal whether an account exists. It's a bcrypt hash of a random
// string generated at startup.
var dummyHash = mustHash(randomToken())

// HashPassword returns a bcrypt hash of the given password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether password matches hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// CheckDummy runs a throwaway comparison to equalize timing on the
// user-not-found path. The result is intentionally ignored by callers.
func CheckDummy(password string) {
	_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
}

// NewSessionID returns a cryptographically random, URL-safe session token.
func NewSessionID() string {
	return randomToken()
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func mustHash(s string) string {
	h, err := HashPassword(s)
	if err != nil {
		panic(err)
	}
	return h
}
