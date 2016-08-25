package main

import "net/http"
import "errors"

import pc "github.com/maklesoft/padlock-cloud/padlockcloud"

var ErrInvalidReceipt = errors.New("padlock: invalid receipt")

type Server struct {
	*http.ServeMux
	*pc.Server
	Itunes ItunesInterface
}

func (server *Server) UpdateSubscriptionsForAccount(acc *SubscriptionAccount) error {
	if acc.ItunesSubscription != nil {
		// Revalidate itunes receipt to see if the subscription has been renewed
		subscription, err := server.Itunes.ValidateReceipt(acc.ItunesSubscription.Receipt)
		if err != nil {
			return err
		}

		acc.ItunesSubscription = subscription
		if err := server.Put(acc); err != nil {
			return err
		}

		// If the itunes subscription has been renewed then we can stop right here
		if acc.ItunesSubscription.Active() {
			return nil
		}
	}

	return nil
}

func (server *Server) CheckSubscriptionsForAccount(acc *SubscriptionAccount) (bool, error) {
	if acc.HasActiveSubscription() {
		return true, nil
	}

	if err := server.UpdateSubscriptionsForAccount(acc); err != nil {
		return false, err
	}

	return acc.HasActiveSubscription(), nil
}

func (server *Server) CheckSubscription(email string, w http.ResponseWriter, r *http.Request) {
	// Get subscription account for this email
	acc := &SubscriptionAccount{Email: email}

	// Load existing data for this subscription account
	if err := server.Get(acc); err == pc.ErrNotFound {
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

func (server *Server) ValidateReceipt(w http.ResponseWriter, r *http.Request) {
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
	if err := server.Get(acc); err != nil && err != pc.ErrNotFound {
		server.HandleError(err, w, r)
	}

	switch receiptType {
	case ReceiptTypeItunes:
		// Validate receipt
		subscription, err := server.Itunes.ValidateReceipt(receiptData)
		// If the receipt is invalid or the subcription expired, return the appropriate error
		if err == ErrInvalidReceipt || subscription.Status == ItunesStatusExpired {
			http.Error(w, "{\"error\": \"invalid_receipt\"}", http.StatusBadRequest)
			return
		}

		if err != nil {
			server.HandleError(err, w, r)
			return
		}

		// Save the subscription with the corresponding account
		acc.ItunesSubscription = subscription
		if err := server.Put(acc); err != nil {
			server.HandleError(err, w, r)
			return
		}
	default:
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (server *Server) SetupRoutes() {
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

func (server *Server) Init() {
	server.SetupRoutes()
}

func (server *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func NewServer(pcServer *pc.Server, itunes ItunesInterface) *Server {
	// Initialize server instance
	server := &Server{
		http.NewServeMux(),
		pcServer,
		itunes,
	}
	server.Init()
	return server
}

func init() {
	pc.AddStorable(&SubscriptionAccount{}, "subscription-accounts")
}
