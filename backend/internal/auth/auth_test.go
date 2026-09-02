package auth

import (
	"testing"

	"github.com/ysnb/oj/internal/model"
)

func TestJWTIssueAndParse(t *testing.T) {
	m := NewManager("test-secret-not-a-real-credential", 1)
	user := &model.User{ID: 7, Username: "alice", Role: model.RoleUser}
	token, err := m.Issue(user)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != 7 || claims.Username != "alice" || claims.Role != model.RoleUser {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

func TestJWTTamperedTokenRejected(t *testing.T) {
	m := NewManager("test-secret-not-a-real-credential", 1)
	other := NewManager("a-different-secret", 1)
	token, _ := m.Issue(&model.User{ID: 1, Username: "bob", Role: model.RoleAdmin})
	if _, err := other.Parse(token); err == nil {
		t.Fatal("token signed with another secret must be rejected")
	}
	if _, err := m.Parse(token + "x"); err == nil {
		t.Fatal("tampered token must be rejected")
	}
	if _, err := m.Parse("not.a.token"); err == nil {
		t.Fatal("garbage token must be rejected")
	}
}

func TestJWTExpiry(t *testing.T) {
	m := NewManager("test-secret-not-a-real-credential", -1) // already expired
	token, _ := m.Issue(&model.User{ID: 1, Username: "carol", Role: model.RoleUser})
	if _, err := m.Parse(token); err == nil {
		t.Fatal("expired token must be rejected")
	}
}

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("hash must not equal plaintext")
	}
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Fatal("correct password must verify")
	}
	if CheckPassword(hash, "wrong password") {
		t.Fatal("wrong password must not verify")
	}
}
