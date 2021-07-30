package backoff

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/utilitywarehouse/semaphore-wireguard/log"
)

var testFuncCallCounter int
var successThreshold int

func testFunc() error {
	testFuncCallCounter++

	if testFuncCallCounter >= successThreshold {
		return nil
	}
	return errors.New("error")

}

func TestRetryWithBackoff(t *testing.T) {
	log.InitLogger("retry-test", "info")
	b := &Backoff{
		Jitter: false,
		Min:    10 * time.Millisecond,
		Max:    1 * time.Second,
	}
	successThreshold = 3

	// Retrying testFunc should fail 2 times before hitting the success
	// threshold
	RetryWithBackoff(testFunc, b, "test func")
	assert.Equal(t, testFuncCallCounter, 3)            // should be 3 after 2 consecutive fails
	assert.Equal(t, b.Duration(), 40*time.Millisecond) // should be 40 millisec after failing for 10 and 20 and without a jitter
}
