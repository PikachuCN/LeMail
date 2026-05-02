package auth

import "testing"

func TestPasswordHashAndSessionValidation(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Fatal("expected password to validate")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("wrong password should not validate")
	}
	sessions := NewSessionManager()
	token := sessions.Create("admin", "root", 0)
	if !sessions.Validate(token, "admin") {
		t.Fatal("expected session to validate")
	}
	if sessions.Validate(token, "access") {
		t.Fatal("session kind should be enforced")
	}
}
