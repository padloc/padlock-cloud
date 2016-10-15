package main

import "time"

type Plan struct {
	Created time.Time
	Expires time.Time
}

func (s *Plan) Active() bool {
	return s.Expires.After(time.Now())
}

type FreePlan struct {
	*Plan
}

type ItunesPlan struct {
	*Plan
	Receipt string
	Status  int
}
