package scheduler

import (
	"errors"
	"testing"
	"time"

	"openpt/internal/clientemu"
)

func TestNextAfterStateMachine(t *testing.T) {
	interval := 30 * time.Second
	start := NextAfter(clientemu.EventStarted, interval, nil)
	if start.NextEvent != clientemu.EventNone || start.Delay != interval || start.Done {
		t.Fatalf("started result = %+v", start)
	}
	retry := NextAfter(clientemu.EventStarted, interval, errors.New("boom"))
	if retry.NextEvent != clientemu.EventStarted || retry.Delay != interval || retry.Done {
		t.Fatalf("retry result = %+v", retry)
	}
	stop := NextAfter(clientemu.EventStopped, interval, nil)
	if !stop.Done {
		t.Fatalf("stop result = %+v", stop)
	}
}
