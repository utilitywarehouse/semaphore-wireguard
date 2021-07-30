package backoff

import (
	"time"

	"github.com/utilitywarehouse/semaphore-wireguard/log"
)

type Operation func() error

const (
	defaultBackoffJitter = true
	defaultBackoffMin    = 2 * time.Second
	defaultBackoffMax    = 1 * time.Minute
)

func RetryWithDefaultBackoff(op Operation, description string) {
	b := &Backoff{
		Jitter: defaultBackoffJitter,
		Min:    defaultBackoffMin,
		Max:    defaultBackoffMax,
	}
	RetryWithBackoff(op, b, description)
}

func RetryWithBackoff(op Operation, b *Backoff, description string) {
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
