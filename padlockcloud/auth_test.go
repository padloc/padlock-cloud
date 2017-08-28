package padlockcloud

import "testing"
import "fmt"

func TestAuthTokenFromString(t *testing.T) {
	token, err := NewAuthToken("martin@padlock.io", "api", nil)
	str := token.String()
	token2, err := AuthTokenFromString(str)
	if err != nil || token.Email != token2.Email || token.Token != token2.Token {
		t.Fatal("Token should be parsed correctly from string")
	}

	if token, err := AuthTokenFromString(""); token != nil || err == nil {
		t.Fatal("Trying to get an auth token from an empty string should fail with an error")
	}

	if token, err := AuthTokenFromString(
		fmt.Sprintf("AuthToken %s:%s", "martin@padlock.io", "asdf"),
	); err != nil || token.Email != "martin@padlock.io" {
		t.Fatal("If email is not base64 encoded, authtoken is still parsed correctly")
	}
}

func TestManageAuthTokens(t *testing.T) {
	acc := &Account{}
	t1, _ := NewAuthToken("martin@padlock.io", "api", &Device{UUID: "uuid123"})

	if acc.AddAuthToken(t1); len(acc.AuthTokens) != 1 || acc.AuthTokens[0] != t1 {
		t.Fatal("Add auth token")
	}

	t2 := &AuthToken{Token: t1.Token}

	if i, t3 := acc.findAuthToken(t2); i != 0 || t1 != t3 {
		t.Fatal("Find auth token by token value")
	}

	t2.Token = ""
	t2.Id = t1.Id
	if i, t3 := acc.findAuthToken(t2); i != 0 || t1 != t3 {
		t.Fatal("Find auth token by id")
	}

	t2.Id = ""
	t2.Device = &Device{UUID: t1.Device.UUID}
	if i, t3 := acc.findAuthToken(t2); i != 0 || t1 != t3 {
		t.Fatal("Find auth token by device uuid")
	}

	t2.Id = "asdf"
	if i, t3 := acc.findAuthToken(t2); i != -1 || t3 != nil {
		t.Fatal("Fail to find invalid auth token")
	}

	t2.Id = t1.Id
	t2.Email = "asdf"
	if acc.UpdateAuthToken(t2); t1.Email != "asdf" {
		t.Fatal("Update auth token")
	}

	if acc.RemoveAuthToken(t2); len(acc.AuthTokens) != 0 {
		t.Fatal("Remove auth token")
	}
}

func TestValidateAuthToken(t *testing.T) {
	acc := &Account{}
	t1, _ := NewAuthToken("asdf", "api", nil)
	t2, _ := NewAuthToken("fsda", "api", nil)
	acc.AddAuthToken(t1)

	if t2.Validate(acc) {
		t.Fatal("Validate should fail")
	}

	if t2.Id = t1.Id; t2.Validate(acc) {
		t.Fatal("Validate should fail")
	}

	t2.Token = t1.Token

	if !t2.Validate(acc) {
		t.Fatal("Validate should return true")
	}

	if t2.Email != t1.Email {
		t.Fatal("Validated token should take on value of original token")
	}

	if t2.account != acc {
		t.Fatal("account field should be set after validation")
	}
}
