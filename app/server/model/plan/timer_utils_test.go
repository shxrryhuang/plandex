package plan

import (
	"testing"
	"time"
)

// TestNonBlockingTimerDrain verifies the non-blocking timer drain pattern
// that was fixed to prevent deadlocks when resetting timers.
func TestNonBlockingTimerDrain(t *testing.T) {
	t.Run("drain works when timer has not fired", func(t *testing.T) {
		timer := time.NewTimer(1 * time.Hour) // Won't fire during test
		defer timer.Stop()

		// This should not block - timer hasn't fired
		done := make(chan bool, 1)
		go func() {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			done <- true
		}()

		select {
		case <-done:
			// Success - didn't block
		case <-time.After(100 * time.Millisecond):
			t.Error("Timer drain blocked when timer had not fired")
		}
	})

	t.Run("drain works when timer has fired and not yet read", func(t *testing.T) {
		timer := time.NewTimer(1 * time.Millisecond)
		time.Sleep(10 * time.Millisecond) // Let timer fire

		done := make(chan bool, 1)
		go func() {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			done <- true
		}()

		select {
		case <-done:
			// Success - drained the fired timer
		case <-time.After(100 * time.Millisecond):
			t.Error("Timer drain blocked when timer had fired")
		}
	})

	t.Run("drain works when timer has fired and already read", func(t *testing.T) {
		timer := time.NewTimer(1 * time.Millisecond)
		time.Sleep(10 * time.Millisecond) // Let timer fire
		<-timer.C                          // Read the timer (simulating select case)

		// Now the channel is empty - this is the scenario that caused deadlock
		done := make(chan bool, 1)
		go func() {
			if !timer.Stop() {
				// OLD CODE (would deadlock):
				// <-timer.C

				// NEW CODE (non-blocking):
				select {
				case <-timer.C:
				default:
				}
			}
			done <- true
		}()

		select {
		case <-done:
			// Success - didn't block even though channel was already drained
		case <-time.After(100 * time.Millisecond):
			t.Error("Timer drain blocked when timer channel was already empty - this is the bug we fixed!")
		}
	})

	t.Run("timer can be reset after non-blocking drain", func(t *testing.T) {
		timer := time.NewTimer(1 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)
		<-timer.C // Drain via select

		// Non-blocking drain
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		// Reset should work
		timer.Reset(50 * time.Millisecond)

		select {
		case <-timer.C:
			// Timer fired after reset - success
		case <-time.After(200 * time.Millisecond):
			t.Error("Timer did not fire after reset")
		}
	})
}

// TestFirstTokenTimeout verifies the timeout calculation for first token
func TestFirstTokenTimeout(t *testing.T) {
	tests := []struct {
		name         string
		tokens       int
		isLocalModel bool
		minExpected  time.Duration
		maxExpected  time.Duration
	}{
		{
			name:         "small request",
			tokens:       10000,
			isLocalModel: false,
			minExpected:  90 * time.Second,
			maxExpected:  90 * time.Second,
		},
		{
			name:         "at step boundary",
			tokens:       150000,
			isLocalModel: false,
			minExpected:  90 * time.Second,
			maxExpected:  90 * time.Second,
		},
		{
			name:         "above step boundary",
			tokens:       300000,
			isLocalModel: false,
			minExpected:  90 * time.Second,
			maxExpected:  180 * time.Second,
		},
		{
			name:         "local model gets max timeout",
			tokens:       10000,
			isLocalModel: true,
			minExpected:  15 * time.Minute,
			maxExpected:  15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firstTokenTimeout(tt.tokens, tt.isLocalModel)
			if result < tt.minExpected || result > tt.maxExpected {
				t.Errorf("firstTokenTimeout(%d, %v) = %v, want between %v and %v",
					tt.tokens, tt.isLocalModel, result, tt.minExpected, tt.maxExpected)
			}
		})
	}
}
