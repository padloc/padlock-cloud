package padlockcloud

import "testing"

func TestValidateAuthToken(t *testing.T) {
	acc := &Account{}
	token, err := NewAuthToken("")
	if err != nil {
		t.Fatal(err)
	}

	acc.AddAuthToken(token)

	lastUsed := token.LastUsed

	if valid := acc.ValidateAuthToken(token.Token); !valid {
		t.Fatal("Validating a valid auth token should return true")
	}

	if !token.LastUsed.After(lastUsed) {
		t.Fatal("LastUsed field should have been updated during validation")
	}

	if valid := acc.ValidateAuthToken(""); valid {
		t.Fatal("Validating invalid auth token should return false")
	}
}
