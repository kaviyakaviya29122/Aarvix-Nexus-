package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	Role  string `json:"role"`
	Email string `json:"email"`
	jwt.RegisteredClaims
}

func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), 12)
	return string(b), err
}

func CheckPassword(hash, p string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

// ValidPassword: min 10 chars, at least one letter and one digit (bcrypt caps input at 72 bytes).
func ValidPassword(p string) error {
	if len(p) < 10 || len(p) > 72 {
		return errors.New("password must be 10-72 characters")
	}
	var letter, digit bool
	for _, r := range p {
		letter = letter || unicode.IsLetter(r)
		digit = digit || unicode.IsDigit(r)
	}
	if !letter || !digit {
		return errors.New("password must contain a letter and a digit")
	}
	return nil
}

// NewCode returns a 6-digit one-time verification code from a CSPRNG.
func NewCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func HashCode(code string) string {
	s := sha256.Sum256([]byte(code))
	return hex.EncodeToString(s[:])
}

func CodeMatches(storedHash, code string) bool {
	return subtle.ConstantTimeCompare([]byte(storedHash), []byte(HashCode(code))) == 1
}

func Sign(secret []byte, uid, email, role string, ttl time.Duration) (string, error) {
	now := time.Now()
	c := Claims{
		Role: role, Email: email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: uid, Issuer: "aarvix-nexus",
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
}

func Parse(secret []byte, tok string) (*Claims, error) {
	c := &Claims{}
	t, err := jwt.ParseWithClaims(tok, c, func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("aarvix-nexus"))
	if err != nil || !t.Valid {
		return nil, errors.New("invalid token")
	}
	return c, nil
}
