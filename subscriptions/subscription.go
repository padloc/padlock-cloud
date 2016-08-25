package main

import "time"
import "encoding/json"

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
