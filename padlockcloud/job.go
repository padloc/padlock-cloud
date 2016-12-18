package padlockcloud

import "time"

type Job struct {
	Action func()
	stop   chan bool
}

func (j *Job) Start(interval time.Duration) {
	j.stop = make(chan bool)
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				j.Action()
			case <-j.stop:
				ticker.Stop()
				return
			}
		}
	}()
}

func (j *Job) Stop() {
	j.stop <- true
}
