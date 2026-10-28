package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", errors.New("WEB_PASSWORD must contain 12 to 1024 bytes")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$600000$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func parseHash(encoded string) (int, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	invalid := errors.New("WEB_PASSWORD_HASH must use pbkdf2-sha256 with 600000 to 2000000 iterations, a 16-byte salt and a 32-byte key")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return 0, nil, nil, invalid
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 600000 || iterations > 2000000 {
		return 0, nil, nil, invalid
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) != 16 {
		return 0, nil, nil, invalid
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(key) != 32 {
		return 0, nil, nil, invalid
	}
	return iterations, salt, key, nil
}

func ValidateHash(encoded string) error { _, _, _, err := parseHash(encoded); return err }

func CheckPassword(encoded, password string) bool {
	if len(password) > 1024 {
		return false
	}
	iterations, salt, expected, err := parseHash(encoded)
	if err != nil {
		return false
	}
	actual, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	return err == nil && subtle.ConstantTimeCompare(actual, expected) == 1
}
