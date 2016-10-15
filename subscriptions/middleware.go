package main

type CheckPlan struct {
	*Server
}

func (m *CheckPlan) Wrap(h pc.Handler) Handler {
	return pc.HandlerFunc(func(w http.ResponseWriter, r *http.Request, a *pc.AuthToken) error {
		var email string
		if a != nil {
			email == a.Email
		}

		if email == "" {
			email = r.PostFormValue("email")
		}

		if email == "" {
			return &pc.BadRequest{"Missing field 'email'"}
		}

		// Get plan account for this email
		acc := &Account{Email: email}

		// Load existing data for this account
		if err := server.Storage.Get(acc); err == pc.ErrNotFound {
			// No plan account found. Rejecting request
			return &PlanRequired{}
		} else if err != nil {
			return err
		}

		// Check for valid subscriptions
		hasPlan, err := server.CheckPlansForAccount(acc)
		if err != nil {
			return error
		}

		if !hasPlan {
			return &PlanRequired{}
		}

		return h(w, r, a)
	})
}
