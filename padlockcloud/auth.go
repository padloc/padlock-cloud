package padlockcloud

import "time"
import "encoding/json"

// A wrapper for an api key containing some meta info like the user and device name
type AuthToken struct {
	Email    string
	Token    string
	Id       string
	Created  time.Time
	LastUsed time.Time
}

// Creates a new auth token for a given `email`
func NewAuthToken(email string) (*AuthToken, error) {
	authT, err := token()
	if err != nil {
		return nil, err
	}
	id, err := randomBase64(6)
	if err != nil {
		return nil, err
	}

	return &AuthToken{
		email,
		authT,
		id,
		time.Now(),
		time.Now(),
	}, nil
}

// A struct representing a user with a set of api keys
type Account struct {
	// The email servers as a unique identifier and as a means for
	// requesting/activating api keys
	Email string
	// A set of api keys that can be used to access the data associated with this
	// account
	AuthTokens []*AuthToken
}

// Implements the `Key` method of the `Storable` interface
func (acc *Account) Key() []byte {
	return []byte(acc.Email)
}

// Implementation of the `Storable.Deserialize` method
func (acc *Account) Deserialize(data []byte) error {
	return json.Unmarshal(data, acc)
}

// Implementation of the `Storable.Serialize` method
func (acc *Account) Serialize() ([]byte, error) {
	return json.Marshal(acc)
}

// Adds an api key to this account. If an api key for the given device
// is already registered, that one will be replaced
func (a *Account) AddAuthToken(token *AuthToken) {
	a.AuthTokens = append(a.AuthTokens, token)
}

// Checks if a given api key is valid for this account
func (a *Account) ValidateAuthToken(token string) bool {
	// Check if the account contains any AuthToken with that matches
	// the given key
	for _, authToken := range a.AuthTokens {
		if authToken.Token == token {
			authToken.LastUsed = time.Now()
			return true
		}
	}

	return false
}

// AuthRequest represents an api key - activation token pair used to activate a given api key
// `AuthRequest.Token` is used to activate the AuthToken through a separate channel (e.g. email)
type AuthRequest struct {
	Token     string
	AuthToken AuthToken
	Created   time.Time
}

// Implementation of the `Storable.Key` interface method
func (ar *AuthRequest) Key() []byte {
	return []byte(ar.Token)
}

// Implementation of the `Storable.Deserialize` method
func (ar *AuthRequest) Deserialize(data []byte) error {
	return json.Unmarshal(data, ar)
}

// Implementation of the `Storable.Serialize` method
func (ar *AuthRequest) Serialize() ([]byte, error) {
	return json.Marshal(ar)
}

// Creates a new `AuthRequest` with a given `email`
func NewAuthRequest(email string) (*AuthRequest, error) {
	// Create new auth token
	authToken, err := NewAuthToken(email)
	if err != nil {
		return nil, err
	}

	// Create activation token
	actToken, err := token()
	if err != nil {
		return nil, err
	}

	return &AuthRequest{actToken, *authToken, time.Now()}, nil
}

func init() {
	RegisterStorable(&Account{}, "auth-accounts")
	RegisterStorable(&AuthRequest{}, "auth-requests")
}
