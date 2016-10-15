package main

import "net/http"
import "errors"

import pc "github.com/maklesoft/padlock-cloud/padlockcloud"

var ErrInvalidReceipt = errors.New("padlock: invalid receipt")

type Server struct {
	mux *http.ServeMux
	*pc.Server
	Itunes ItunesInterface
}

func (server *Server) UpdatePlansForAccount(acc *PlanAccount) error {
	if acc.Plans.Itunes != nil {
		// Revalidate itunes receipt to see if the plan has been renewed
		plan, err := server.Itunes.ValidateReceipt(acc.Plans.Itunes.Receipt)
		if err != nil {
			return err
		}

		acc.Plans.Itunes = plan
		if err := server.Storage.Put(acc); err != nil {
			return err
		}

		// If the itunes plan has been renewed then we can stop right here
		if acc.Plans.Itunes.Active() {
			return nil
		}
	}

	return nil
}

func (server *Server) CheckPlansForAccount(acc *PlanAccount) (bool, error) {
	if acc.HasActivePlan() {
		return true, nil
	}

	if err := server.UpdatePlansForAccount(acc); err != nil {
		return false, err
	}

	return acc.HasActivePlan(), nil
}

func (server *Server) InitEndpoints() {
	auth := server.Server.Endpoints["/auth/"]
	auth.Handlers["POST"] = &CheckPlan{server}.Wrap(auth.Handlers["POST"])
	auth.Handlers["PUT"] = &CheckPlan{server}.Wrap(auth.Handlers["PUT"])

	// Endpoint for validating purchase receipts, only POST method is supported
	server.Server.Endpoints["/validatereceipt/"] = &Endpoint{
		Handlers: map[string]Handler{
			"POST": &ValidateReceipt{server},
		},
	}
}

func (server *Server) Init() error {
	if err := server.Server.Init(); err != nil {
		return err
	}
	server.InitEndpoints()
	return nil
}

func (server *Server) Start() error {
	if err := server.Init(); err != nil {
		return err
	}
	defer server.CleanUp()

	var handler http.Handler = server.mux

	// Add CORS middleware
	handler = Cors(handler)

	// Add panic recovery
	handler = server.HandlePanic(handler)

	server.ServeHandler(handler)
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
	pc.AddStorable(&PlanAccount{}, "plan-accounts")
}
