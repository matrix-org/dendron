package login

import (
	"fmt"
	"testing"
)

const (
	testUserID         = "@test:example.org"
	testPasswordBcrypt = "$2a$12$Qc4ztcl9b29JV5J1pEh3DeGwwX05OcaP0Hw0pQYL8Nop1g0cjPv.u" // bcrypt("test_password")
)

func TestGoodPassword(t *testing.T) {
	db := mockDatabase{
		passwords: map[string]string{
			testUserID: testPasswordBcrypt,
		},
	}

	h := &MatrixLoginHandler{
		db:             &db,
		serverName:     "example.org",
		macaroonSecret: "test_secret",
	}

	r, err := h.loginPassword(testUserID, "test_password")
	if err != nil {
		t.Fatal(err)
	}

	if len(db.accessTokens) != 1 {
		t.Fatalf("Want 1 access token, got %v", db.accessTokens)
	}

	if len(db.refreshTokens) != 1 {
		t.Fatalf("Want 1 refresh token, got %v", db.refreshTokens)
	}

	if db.accessTokens[0].token != r.AccessToken {
		t.Errorf("AccessToken: Want %v got %v", db.accessTokens[0], r.AccessToken)
	}

	if db.refreshTokens[0].token != r.RefreshToken {
		t.Errorf("RefreshToken: Want %v got %v", db.refreshTokens[0], r.RefreshToken)
	}

	if db.accessTokens[0].userID != testUserID {
		t.Errorf("Inserted access token: Want %v got %v", testUserID, db.accessTokens[0].userID)
	}

	if db.refreshTokens[0].userID != testUserID {
		t.Errorf("Inserted refresh token: Want %v got %v", testUserID, db.refreshTokens[0].userID)
	}

	if r.UserID != testUserID {
		t.Errorf("UserID: Want %v got %v", testUserID, r.UserID)
	}
}

func TestBadPassword(t *testing.T) {
	db := mockDatabase{
		passwords: map[string]string{
			testUserID: testPasswordBcrypt,
		},
	}
	testExpectLoginFailure(t, &db)
}

func TestUnkownUserID(t *testing.T) {
	db := mockDatabase{}
	testExpectLoginFailure(t, &db)
}

func testExpectLoginFailure(t *testing.T, db *mockDatabase) {
	h := &MatrixLoginHandler{
		db:             db,
		serverName:     "example.org",
		macaroonSecret: "test_secret",
	}

	_, err := h.loginPassword(testUserID, "bad_password")
	if err == nil {
		t.Fatal("Want error got nil")
	}

	if len(db.accessTokens) != 0 {
		t.Fatalf("Want 0 access token, got %v", db.accessTokens)
	}

	if len(db.refreshTokens) != 0 {
		t.Fatalf("Want 0 refresh token, got %v", db.refreshTokens)
	}
}

type tokenRow struct {
	token  string
	userID string
}

type mockDatabase struct {
	passwords     map[string]string
	accessTokens  []tokenRow
	refreshTokens []tokenRow
}

func (m *mockDatabase) passwordHash(userID string) (string, error) {
	if val, ok := m.passwords[userID]; ok {
		return val, nil
	}
	return "", fmt.Errorf("no such userID: %s", userID)
}

func (m *mockDatabase) insertTokens(userID, accessToken, refreshToken string) error {
	m.accessTokens = append(m.accessTokens, tokenRow{accessToken, userID})
	m.refreshTokens = append(m.refreshTokens, tokenRow{refreshToken, userID})
	return nil
}
