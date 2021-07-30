package backoff

import (
	"time"

	"github.com/utilitywarehouse/semaphore-wireguard/log"
)

type operation func() error

const (
	defaultBackoffJitter = true
	defaultBackoffMin    = 2 * time.Second
	defaultBackoffMax    = 1 * time.Minute
)

// Retry will use the default backoff values to retry the passed operation
func Retry(op operation, description string) {
	b := &Backoff{
		Jitter: defaultBackoffJitter,
		Min:    defaultBackoffMin,
		Max:    defaultBackoffMax,
	}
	RetryWithBackoff(op, b, description)
}

// RetryWithBackoff will retry the passed function (operation) using the given
// backoff
func RetryWithBackoff(op operation, b *Backoff, description string) {
	b.Reset()
	for {
		err := op()
		if err == nil {
			return
		}
		d := b.Duration()
		log.Logger.Error("Retry failed",
			"description", description,
			"error", err,
			"backoff", d,
		)
		time.Sleep(d)
	}
}
