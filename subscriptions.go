package main

import "net/http"
import "fmt"
import "encoding/json"
import "bytes"
import "io/ioutil"
import "time"
import "strconv"
import "errors"

const (
	ReceiptTypeItunes  = "ios-appstore"
	ReceiptTypeAndroid = "android-playstore"
)

const (
	ItunesStatusOK                   = 0
	ItunesStatusInvalidJSON          = 21000
	ItunesStatusInvalidReceipt       = 21002
	ItunesStatusNotAuthenticated     = 21003
	ItunesStatusWrongSecret          = 21004
	ItunesStatusServerUnavailable    = 21005
	ItunesStatusExpired              = 21006
	ItunesStatusWrongEnvironmentProd = 21007
	ItunesStatusWrongEnvironmentTest = 21008
)

var ErrInvalidReceipt = errors.New("padlock: invalid receipt")

type ItunesSubscription struct {
	Receipt string
	Expires time.Time
	Status  int
}

func (s *ItunesSubscription) Active() bool {
	return s.Expires.After(time.Now())
}

type FreeSubscription struct {
	Expires time.Time
}

func (s *FreeSubscription) Active() bool {
	return s.Expires.After(time.Now())
}

type SubscriptionAccount struct {
	Email              string
	ItunesSubscription *ItunesSubscription
	FreeSubscription   *FreeSubscription
}

// Implements the `Key` method of the `Storable` interface
func (acc *SubscriptionAccount) Key() []byte {
	return []byte(acc.Email)
}

// Implementation of the `Storable.Deserialize` method
func (acc *SubscriptionAccount) Deserialize(data []byte) error {
	return json.Unmarshal(data, acc)
}

// Implementation of the `Storable.Serialize` method
func (acc *SubscriptionAccount) Serialize() ([]byte, error) {
	return json.Marshal(acc)
}

func (acc *SubscriptionAccount) HasActiveSubscription() bool {
	return (acc.FreeSubscription != nil && acc.FreeSubscription.Active()) ||
		(acc.ItunesSubscription != nil && acc.ItunesSubscription.Active())
}

type SubscriptionServerConfig struct {
	ItunesSharedSecret string `yaml:"itunes_shared_secret"`
	ItunesEnvironment  string `yaml:"itunes_environment"`
}

type SubscriptionServer struct {
	*http.ServeMux
	*Server
	SubscriptionServerConfig
}

func (server *SubscriptionServer) ValidateItunesReceipt(acc *SubscriptionAccount) error {
	body, err := json.Marshal(map[string]string{
		"receipt-data": acc.ItunesSubscription.Receipt,
		"password":     server.ItunesSharedSecret,
	})
	if err != nil {
		return err
	}

	var itunesUrl string

	if server.ItunesEnvironment == "production" {
		itunesUrl = "https://buy.itunes.apple.com/verifyReceipt"
	} else {
		itunesUrl = "https://sandbox.itunes.apple.com/verifyReceipt"
	}

	resp, err := http.Post(itunesUrl, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	result := &struct {
		Status            int `json:"status"`
		LatestReceiptInfo struct {
			Expires string `json:"expires_date"`
		} `json:"latest_receipt_info"`
		LatestReceipt string `json:"latest_receipt"`
	}{}
	err = json.Unmarshal(raw, result)
	if err != nil {
		return err
	}

	if result.Status != ItunesStatusOK {
		switch result.Status {
		case ItunesStatusInvalidReceipt, ItunesStatusNotAuthenticated, ItunesStatusExpired:
			return ErrInvalidReceipt
		default:
			return errors.New(fmt.Sprintf("Failed to validate receipt, status: %d", result.Status))
		}
	}

	ts, err := strconv.Atoi(result.LatestReceiptInfo.Expires)
	if err != nil {
		return err
	}

	acc.ItunesSubscription.Status = result.Status
	acc.ItunesSubscription.Expires = time.Unix(0, int64(ts)*1000000)
	acc.ItunesSubscription.Receipt = result.LatestReceipt

	if err := server.Put(acc); err != nil {
		return err
	}

	return nil
}

func (server *SubscriptionServer) UpdateSubscriptionsForAccount(acc *SubscriptionAccount) error {
	if acc.ItunesSubscription != nil {
		// Revalidate itunes receipt to see if the subscription has been renewed
		if err := server.ValidateItunesReceipt(acc); err != nil {
			return err
		}

		// If the itunes subscription has been renewed then we can stop right here
		if acc.ItunesSubscription.Active() {
			return nil
		}
	}

	return nil
}

func (server *SubscriptionServer) CheckSubscriptionsForAccount(acc *SubscriptionAccount) (bool, error) {
	if acc.HasActiveSubscription() {
		return true, nil
	}

	if err := server.UpdateSubscriptionsForAccount(acc); err != nil {
		return false, err
	}

	return acc.HasActiveSubscription(), nil
}

func (server *SubscriptionServer) CheckSubscription(email string, w http.ResponseWriter, r *http.Request) {
	// Get subscription account for this email
	acc := &SubscriptionAccount{Email: email}

	// Load existing data for this subscription account
	if err := server.Get(acc); err == ErrNotFound {
		// No subscription account found. Rejecting request
		http.Error(w, "", http.StatusPaymentRequired)
	} else if err != nil {
		// Some other error
		server.HandleError(err, w, r)
		return
	}

	// Check for valid subscriptions
	hasSubscription, err := server.CheckSubscriptionsForAccount(acc)
	if err != nil {
		server.HandleError(err, w, r)
		return
	}

	// If the account has a valid subscription, forward the request. otherwise reject it.
	if hasSubscription {
		server.Server.ServeHTTP(w, r)
	} else {
		http.Error(w, "", http.StatusPaymentRequired)
	}
}

func (server *SubscriptionServer) ValidateReceipt(w http.ResponseWriter, r *http.Request) {
	receiptType := r.PostFormValue("type")
	receiptData := r.PostFormValue("receipt")
	email := r.PostFormValue("email")

	// Make sure all required parameters are there
	if email == "" || receiptType == "" || receiptData == "" {
		http.Error(w, "", http.StatusBadRequest)
	}

	acc := &SubscriptionAccount{Email: email}

	// Load existing account data if there is any. If not, that's fine, one will be created later
	// if the receipt turns out fine
	if err := server.Get(acc); err != nil && err != ErrNotFound {
		server.HandleError(err, w, r)
	}

	switch receiptType {
	case ReceiptTypeItunes:
		acc.ItunesSubscription = &ItunesSubscription{Receipt: receiptData}
		err := server.ValidateItunesReceipt(acc)

		if err == ErrInvalidReceipt {
			http.Error(w, "{\"error\": \"invalid_receipt\"}", http.StatusBadRequest)
			return
		}

		if err != nil {
			server.HandleError(err, w, r)
			return
		}
	default:
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (server *SubscriptionServer) SetupRoutes() {
	server.HandleFunc("/auth/", func(w http.ResponseWriter, r *http.Request) {
		// A subscription is required only for creating new accounts (POST method)
		// Retrieving authentication tokens for existing accounts (PUT) does not
		// require an active subscription
		if r.Method == "POST" {
			email := r.PostFormValue("email")
			server.CheckSubscription(email, w, r)
		} else {
			server.Server.ServeHTTP(w, r)
		}
	})

	server.HandleFunc("/store/", func(w http.ResponseWriter, r *http.Request) {
		// A subscription is only required for updating the remote data (PUT method)
		// Accessing (GET method) or deleting (DELETE) existing data does not require
		// an active subscription
		if r.Method == "PUT" {
			acc, _ := server.AccountFromRequest(r)
			if acc != nil {
				server.CheckSubscription(acc.Email, w, r)
			} else {
				// If the request is not authenticated (acc == nil) we can just pass
				// it on to the data server which will reject the request
				server.Server.ServeHTTP(w, r)
			}
		} else {
			server.Server.ServeHTTP(w, r)
		}
	})

	// Endpoint for validating purchase receipts, only POST method is supported
	server.HandleFunc("/validatereceipt/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			server.ValidateReceipt(w, r)
		// case "GET":
		// 	email := r.URL.Query().Get("email")
		// 	acc := &SubscriptionAccount{Email: email}
		// 	err := server.Get(acc)
		// 	if err != nil {
		// 		server.HandleError(err, w, r)
		// 		return
		// 	}
		// 	raw, _ := json.Marshal(acc)
		// 	w.Write(raw)
		default:
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	server.Handle("/", server.Server)
}

func (server *SubscriptionServer) Init() {
	server.SetupRoutes()
}

func (server *SubscriptionServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer server.HandlePanic(w, r)

	if server.CheckVersion(w, r) {
		server.DeprecatedVersion(w, r)
		return
	}

	// // Temporarily allow circumenting subscription check via 'Require-Subscription' header
	// requireSubscription := r.Header.Get("Require-Subscription")
	// if requireSubscription == "NO" {
	// 	server.Server.ServeHTTP(w, r)
	// 	return
	// }

	server.ServeMux.ServeHTTP(w, r)
}

func NewSubscriptionServer(server *Server, config SubscriptionServerConfig) *SubscriptionServer {
	// Initialize server instance
	subServer := &SubscriptionServer{
		http.NewServeMux(),
		server,
		config,
	}
	subServer.Init()
	return subServer
}

func init() {
	AddStorable(&SubscriptionAccount{}, "subscription-accounts")
}
