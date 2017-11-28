package padlockcloud

import (
	"bufio"
	"fmt"
	"os"
)

type Whitelist struct {
	// Whitelisted emails
	Emails map[string]bool
}

func NewWhitelist(path string) (*Whitelist, error) {
	wl := &Whitelist{}
	if err := wl.init(path); err != nil {
		return nil, err
	}
	return wl, nil
}

func (w *Whitelist) init(path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("Whitelist file[%s] does not exist:.\n", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("Error opening Whitelist file: %s\n", err.Error())
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	w.Emails = make(map[string]bool)
	for scanner.Scan() {
		w.Emails[scanner.Text()] = true
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Error reading in emails, whitelist not set: %s\n", err.Error())
	}

	return nil
}

// Returns whether email is whitelisted or not
func (w *Whitelist) IsWhitelisted(email string) bool {
	return w.Emails[email]
}
