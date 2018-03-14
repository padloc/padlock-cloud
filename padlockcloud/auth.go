package padlockcloud

import "time"
import "encoding/json"
import "encoding/base64"
import "net/http"
import "regexp"
import "fmt"
import "errors"

var authStringPattern = regexp.MustCompile("^(?:AuthToken|ApiKey) (.+):(.+)$")
var authMaxAge = func(authType string) time.Duration {
	switch authType {
	case "web":
		return time.Hour
	default:
		return time.Duration(0)
	}
}

// A wrapper for an api key containing some meta info like the user and device name
type AuthToken struct {
	Email          string
	Token          string
	Type           string
	Id             string
	Created        time.Time
	LastUsed       time.Time
	Expires        time.Time
	ClientVersion  string
	ClientPlatform string
	Device         *Device
	account        *Account
}

// Returns the account associated with this auth token
func (t *AuthToken) Account() *Account {
	return t.account
}

// Validates the auth token against account `a`, i.e. looks for the corresponding
// token in the accounts `AuthTokens` slice. If found, the token is considered valid
// and it's value is updated with the value of the corresponding auth token in `a.AuthTokens`
// and the `account` field is set to `a`
func (t *AuthToken) Validate(a *Account) bool {
	if _, at := a.findAuthToken(t); at != nil && at.Token == t.Token {
		*t = *at
		t.account = a
		return true
	}

	return false
}

// Returns a string representation of the auth token in the form "AuthToken base64(t.Email):t.Token"
func (t *AuthToken) String() string {
	return fmt.Sprintf(
		"AuthToken %s:%s",
		base64.RawURLEncoding.EncodeToString([]byte(t.Email)),
		t.Token,
	)
}

// Returns true if `t` is expires, false otherwise
func (t *AuthToken) Expired() bool {
	return !t.Expires.IsZero() && t.Expires.Before(time.Now())
}

func (t *AuthToken) Description() string {
	if t.Device != nil {
		return t.Device.Description()
	} else if t.ClientPlatform != "" {
		return PlatformDisplayName(t.ClientPlatform) + " Device"
	} else {
		// Older versions of the Padlock Client that didn't send device data
		// were mostly only available on iOS and Android, so "Mobile Device"
		// is the best guess we have in this case
		return "Mobile Device"
	}
}

func (t *AuthToken) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"description": t.Description(),
		"tokenId":     t.Id,
	}
}

// Creates an auth token from it's string representation of the form "AuthToken base64(t.Email):t.Token"
func AuthTokenFromString(str string) (*AuthToken, error) {
	// Check if the Authorization header exists and is well formed
	if !authStringPattern.MatchString(str) {
		return nil, errors.New("invalid credentials")
	}

	// Extract email and auth token from Authorization header
	matches := authStringPattern.FindStringSubmatch(str)
	email := matches[1]
	// Try to decode email in case it's in base64
	if dec, err := base64.RawURLEncoding.DecodeString(matches[1]); err == nil {
		email = string(dec)
	}
	t := &AuthToken{
		Email: email,
		Token: matches[2],
	}
	return t, nil
}

// Creates an auth token from a given request by parsing the `Authorization` header
// and `auth` cookie
func AuthTokenFromRequest(r *http.Request) (*AuthToken, error) {
	authString := r.Header.Get("Authorization")

	if authString == "" {
		if cookie, err := r.Cookie("auth"); err == nil {
			authString = cookie.Value
		}
	}

	return AuthTokenFromString(authString)
}

// Creates a new auth token for a given `email`
func NewAuthToken(email string, t string, device *Device) (*AuthToken, error) {
	authT, err := token()
	if err != nil {
		return nil, err
	}
	id, err := randomBase64(6)
	if err != nil {
		return nil, err
	}

	if t == "" {
		t = "api"
	}

	var expires time.Time

	if maxAge := authMaxAge(t); maxAge != 0 {
		expires = time.Now().Add(maxAge)
	}

	return &AuthToken{
		Email:    email,
		Token:    authT,
		Type:     t,
		Id:       id,
		Created:  time.Now(),
		LastUsed: time.Now(),
		Expires:  expires,
		Device:   device,
	}, nil
}

// A struct representing a user with a set of api keys
type Account struct {
	// The email servers as a unique identifier and as a means for
	// requesting/activating api keys
	Email string
	// Time the account was created
	Created time.Time
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
	if acc.Created.IsZero() {
		acc.Created = time.Now()
	}
	return json.Marshal(acc)
}

// Adds an api key to this account. If an api key for the given device
// is already registered, that one will be replaced
func (a *Account) AddAuthToken(token *AuthToken) {
	a.AuthTokens = append(a.AuthTokens, token)
}

// Returns the matching AuthToken instance by comparing Token field, Id or both
// If either Id or Token field is empty, only the other one will compared. If
// both are empty, nil is returned
func (a *Account) findAuthToken(at *AuthToken) (int, *AuthToken) {
	if at.Token == "" && at.Id == "" && (at.Device == nil || at.Device.UUID == "") {
		return -1, nil
	}
	for i, t := range a.AuthTokens {
		if t != nil &&
			(at.Type == "" || t.Type == at.Type) &&
			(at.Token == "" || t.Token == at.Token) &&
			(at.Id == "" || t.Id == at.Id) &&
			(at.Device == nil || at.Device.UUID == "" || t.Device != nil && t.Device.UUID == at.Device.UUID) {
			return i, t
			fmt.Println(at, i, t)
		}
	}
	return -1, nil
}

// Updates the correspoding auth token in the accounts `AuthTokens` slice with the
// value of `t`
func (a *Account) UpdateAuthToken(t *AuthToken) {
	if _, at := a.findAuthToken(t); at != nil {
		*at = *t
	}
}

// Removes the corresponding auth token from the accounts `AuthTokens` slice
func (a *Account) RemoveAuthToken(t *AuthToken) bool {
	if i, _ := a.findAuthToken(t); i != -1 {
		s := a.AuthTokens
		s[i] = s[len(s)-1]
		s[len(s)-1] = nil
		a.AuthTokens = s[:len(s)-1]
		return true
	}

	return false
}

// Filters out auth tokens that have been expired for 7 days or more
func (a *Account) RemoveExpiredAuthTokens() {
	s := a.AuthTokens[:0]

	for _, t := range a.AuthTokens {
		var maxAge time.Duration = 0
		// Keep expired api tokens around for a while longer
		if t.Type == "api" {
			maxAge = 7 * 24 * time.Hour
		}
		if t.Expires.IsZero() || t.Expires.After(time.Now().Add(-maxAge)) {
			s = append(s, t)
		}
	}

	a.AuthTokens = s
}

// Expires auth tokens that haven't been used in a while
func (a *Account) ExpireUnusedAuthTokens() {
	maxIdle := 30 * 24 * time.Hour
	s := a.AuthTokens[:0]

	for _, t := range a.AuthTokens {
		if t.LastUsed.After(time.Now().Add(-maxIdle)) {
			s = append(s, t)
		}
	}

	a.AuthTokens = s
}

func (a *Account) AuthTokensByType(typ string) []*AuthToken {
	var tokens []*AuthToken
	for _, t := range a.AuthTokens {
		if t != nil && t.Type == typ {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

func (a *Account) Devices() []*AuthToken {
	devices := make([]*AuthToken, 0)
	for _, at := range a.AuthTokensByType("api") {
		if !at.Expired() {
			devices = append(devices, at)
		}
	}
	return devices
}

func (a *Account) ToMap() map[string]interface{} {
	obj := map[string]interface{}{
		"email": a.Email,
	}

	devices := make([]map[string]interface{}, 0)
	for _, at := range a.Devices() {
		devices = append(devices, at.ToMap())
	}

	obj["devices"] = devices
	return obj
}

// AuthRequest represents an api key - activation token pair used to activate a given api key
// `AuthRequest.Token` is used to activate the AuthToken through a separate channel (e.g. email)
type AuthRequest struct {
	Code      string
	Token     string
	AuthToken *AuthToken
	Created   time.Time
	Redirect  string
}

// Implementation of the `Storable.Key` interface method
func (ar *AuthRequest) Key() []byte {
	if ar.Token != "" {
		return []byte(ar.Token)
	} else {
		return []byte(fmt.Sprintf("%s-%s", ar.AuthToken.Email, ar.Code))
	}
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
func NewAuthRequest(email string, tType string, actType string, device *Device) (*AuthRequest, error) {
	var authToken *AuthToken
	var err error

	// Create new auth token
	if authToken, err = NewAuthToken(email, tType, device); err != nil {
		return nil, err
	}

	ar := &AuthRequest{
		AuthToken: authToken,
		Created:   time.Now(),
		Redirect:  "",
	}

	if actType == "code" {
		ar.Code, err = randomHex(3)
	} else {
		ar.Token, err = token()
	}

	if err != nil {
		return nil, err
	}

	return ar, nil
}

func init() {
	RegisterStorable(&Account{}, "auth-accounts")
	RegisterStorable(&AuthRequest{}, "auth-requests")
}
