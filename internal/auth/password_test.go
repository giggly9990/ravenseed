package auth

import "testing"

func TestPasswordHash(t *testing.T) {
	password := "a-long-test-password"
	hash, err := HashPassword(password)
	if err != nil || ValidateHash(hash) != nil {
		t.Fatal("invalid generated hash", err)
	}
	if !CheckPassword(hash, password) || CheckPassword(hash, "wrong-password") {
		t.Fatal("password verification failed")
	}
	for _, invalid := range []string{"plaintext", "pbkdf2-sha256$1$abc$abc", "pbkdf2-sha256$999999999$abc$abc"} {
		if ValidateHash(invalid) == nil || CheckPassword(invalid, password) {
			t.Fatal("invalid hash accepted")
		}
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password accepted")
	}
}

func TestHashFormatBounds(t *testing.T) {
	salt16, salt8 := "AAAAAAAAAAAAAAAAAAAAAA", "AAAAAAAAAAA"
	key32, key16 := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "AAAAAAAAAAAAAAAAAAAAAA"
	for hash, valid := range map[string]bool{
		"pbkdf2-sha256$600000$" + salt16 + "$" + key32:  true,
		"pbkdf2-sha256$2000000$" + salt16 + "$" + key32: true,
		"pbkdf2-sha256$599999$" + salt16 + "$" + key32:  false,
		"pbkdf2-sha256$2000001$" + salt16 + "$" + key32: false,
		"pbkdf2-sha256$600000$" + salt8 + "$" + key32:   false,
		"pbkdf2-sha256$600000$" + salt16 + "$" + key16:  false,
		"pbkdf2-sha512$600000$" + salt16 + "$" + key32:  false,
	} {
		if (ValidateHash(hash) == nil) != valid {
			t.Errorf("ValidateHash(%q) valid=%v", hash, !valid)
		}
	}
	long := make([]byte, 1025)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := HashPassword(string(long)); err == nil {
		t.Fatal("password over 1024 bytes accepted")
	}
	hash, err := HashPassword(string(long[:1024]))
	if err != nil || !CheckPassword(hash, string(long[:1024])) || CheckPassword(hash, string(long)) {
		t.Fatal("1024-byte boundary not enforced")
	}
}
