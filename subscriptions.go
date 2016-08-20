package main

import "net/http"
import "log"
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

func (subscr *ItunesSubscription) ValidateReceipt(config SubscriptionServerConfig) error {
	jsonStr, _ := json.MarshalIndent(subscr, "", "  ")

	body, err := json.Marshal(map[string]string{
		"receipt-data": subscr.Receipt,
		"password":     config.ItunesSharedSecret,
	})
	if err != nil {
		return err
	}

	var itunesUrl string

	if config.ItunesEnvironment == "production" {
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

	jsonStr, _ = json.MarshalIndent(result, "", "  ")
	log.Printf("validation result:\n%s", jsonStr)

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

	subscr.Status = result.Status
	subscr.Expires = time.Unix(0, int64(ts)*1000000)
	subscr.Receipt = result.LatestReceipt

	return nil
}

type SubscriptionAccount struct {
	Email              string
	ItunesSubscription *ItunesSubscription
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

func (acc *SubscriptionAccount) UpdateSubscriptions(config SubscriptionServerConfig) {
	if acc.ItunesSubscription != nil && acc.ItunesSubscription.Expires.Before(time.Now()) {
		log.Printf("Subscription expired. Checking for automated renewal.")
		acc.ItunesSubscription.ValidateReceipt(config)
		jsonStr, _ := json.MarshalIndent(acc.ItunesSubscription, "", "  ")
		log.Printf("Subscription (after update):\n%s", jsonStr)
	}
}

func (acc *SubscriptionAccount) HasActiveSubscription() bool {
	if acc.ItunesSubscription == nil {
		return false
	}

	return acc.ItunesSubscription != nil && acc.ItunesSubscription.Expires.After(time.Now())
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

func (server *SubscriptionServer) ValidateReceipt(w http.ResponseWriter, r *http.Request) {
	receiptType := r.PostFormValue("type")
	receiptData := r.PostFormValue("receipt")
	email := r.PostFormValue("email")
	log.Println("validating receipts", receiptType, receiptData, email)

	if email == "" || receiptType == "" || receiptData == "" {
		http.Error(w, "", http.StatusBadRequest)
	}

	acc := &SubscriptionAccount{Email: email}

	err := server.Get(acc)
	if err != nil && err != ErrNotFound {
		server.HandleError(err, w, r)
	}

	switch receiptType {
	case ReceiptTypeItunes:
		subscr := &ItunesSubscription{Receipt: receiptData}
		err = subscr.ValidateReceipt(server.SubscriptionServerConfig)

		if err == ErrInvalidReceipt {
			http.Error(w, "{\"error\": \"invalid_receipt\"}", http.StatusBadRequest)
			return
		}

		if err != nil {
			server.HandleError(err, w, r)
			return
		}

		acc.ItunesSubscription = subscr
	default:
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	err = server.Put(acc)
	if err != nil {
		server.HandleError(err, w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (server *SubscriptionServer) CheckSubscription(email string, w http.ResponseWriter, r *http.Request) {
	log.Printf("Checking subscription for account %s", email)
	acc := &SubscriptionAccount{Email: email}
	err := server.Get(acc)

	if err != nil && err != ErrNotFound {
		server.HandleError(err, w, r)
		return
	}

	acc.UpdateSubscriptions(server.SubscriptionServerConfig)

	err = server.Put(acc)
	if err != nil {
		server.HandleError(err, w, r)
		return
	}

	if !acc.HasActiveSubscription() {
		http.Error(w, "", http.StatusPaymentRequired)
	} else {
		server.Server.ServeHTTP(w, r)
	}
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
		if r.Method == "PUT" {
			acc, _ := server.AccountFromRequest(r)
			if acc != nil {
				server.CheckSubscription(acc.Email, w, r)
			} else {
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
