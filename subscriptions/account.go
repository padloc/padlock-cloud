package main

import "encoding/json"

type Account struct {
	Email string
	Plans struct {
		Free   *FreePlan
		Itunes *ItunesPlan
	}
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
	return json.Marshal(acc)
}

func (acc *Account) HasActivePlan() bool {
	return (acc.Plans.Free != nil && acc.Plans.Free.Active()) ||
		(acc.Plans.Itunes != nil && acc.Plans.Itunes.Active())
}
