package main

type PlanRequired struct {
}

func (e *PlanRequired) Code() string {
	return "plan_required"
}

func (e *PlanRequired) Error() string {
	return fmt.Sprintf("%s", e.Code())
}

func (e *PlanRequired) Status() int {
	return http.StatusForbidden
}

func (e *PlanRequired) Message() string {
	return fmt.Sprintf("%s: %s", http.StatusText(e.Status()), e.message)
}

type InvalidReceipt struct {
}

func (e *InvalidReceipt) Code() string {
	return "invalid_receipt"
}

func (e *InvalidReceipt) Error() string {
	return fmt.Sprintf("%s", e.Code())
}

func (e *InvalidReceipt) Status() int {
	return http.StatusBadRequest
}

func (e *InvalidReceipt) Message() string {
	return fmt.Sprintf("%s: %s", http.StatusText(e.Status()), e.message)
}
