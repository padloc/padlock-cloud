package main

type ValidateReceipt struct {
	*Server
}

func (h *ValidateReceipt) Handle(w http.ResponseWriter, r *http.Request, a *pc.AuthToken) error {
	receiptType := r.PostFormValue("type")
	receiptData := r.PostFormValue("receipt")
	email := r.PostFormValue("email")

	// Make sure all required parameters are there
	if email == "" || receiptType == "" || receiptData == "" {
		return &pc.BadRequest{"Missing email, receiptType or receiptData field"}
	}

	acc := &Account{Email: email}

	// Load existing account data if there is any. If not, that's fine, one will be created later
	// if the receipt turns out fine
	if err := h.Storage.Get(acc); err != nil && err != pc.ErrNotFound {
		return err
	}

	switch receiptType {
	case ReceiptTypeItunes:
		// Validate receipt
		plan, err := h.Itunes.ValidateReceipt(receiptData)
		// If the receipt is invalid or the subcription expired, return the appropriate error
		if err == ErrInvalidReceipt || plan.Status == ItunesStatusExpired {
			return &pc.BadRequest{"Invalid itunes receipt"}
		}

		if err != nil {
			return err
		}

		// Save the plan with the corresponding account
		acc.Plans.Itunes = plan
		if err := h.Storage.Put(acc); err != nil {
			return err
		}
	default:
		return &pc.BadRequest{"Invalid receipt type"}
	}

	w.WriteHeader(http.StatusNoContent)
}
