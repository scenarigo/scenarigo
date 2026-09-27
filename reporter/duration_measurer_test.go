package reporter

import (
	"sync"
	"testing"
	"time"
)

var (
	_ testDurationMeasurer = &durationMeasurer{}
	_ testDurationMeasurer = &fixedDurationMeasurer{}
)

type fixedDurationMeasurer struct {
	duration time.Duration
}

func (m *fixedDurationMeasurer) start() {
}

func (m *fixedDurationMeasurer) stop() {
}

func (m *fixedDurationMeasurer) spawn() testDurationMeasurer {
	return &fixedDurationMeasurer{
		duration: m.duration,
	}
}

func (m *fixedDurationMeasurer) getDuration() time.Duration {
	return m.duration
}

/*
timeline diagram

	400ms parent
	300ms |-child1
	      | |-child1-1 |----->
	      | |-child1-2 |  ------>
	200ms |-child2     |  |  |  |
	        |-child2-1 |  --->  |
	        |-child2-2 |  |  |  |  --->
	                   |  |  |  |  |  |
	                   0  1  2  3  4  5 (100ms)
*/
func TestDurationMeasurer(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	ch := make(chan struct{})

	parent := &durationMeasurer{}
	child1 := parent.spawn()
	child2 := parent.spawn()

	// child1-1
	wg.Go(func() {
		<-ch
		child1.start()
		time.Sleep(20 * durationTestUnit)
		child1.stop()
	})

	// child1-2
	wg.Go(func() {
		<-ch
		time.Sleep(10 * durationTestUnit)
		child1.start()
		time.Sleep(20 * durationTestUnit)
		child1.stop()
	})

	// child2-1
	wg.Go(func() {
		<-ch
		time.Sleep(10 * durationTestUnit)
		child2.start()
		time.Sleep(10 * durationTestUnit)
		child2.stop()
	})

	// child2-2
	wg.Go(func() {
		<-ch
		time.Sleep(40 * durationTestUnit)
		child2.start()
		time.Sleep(10 * durationTestUnit)
		child2.stop()
	})

	close(ch)
	wg.Wait()

	// Each duration is a union of intervals that start in different
	// goroutines, so a goroutine scheduled late shortens it slightly, while
	// oversleeping lengthens it. Truncating would turn a value just short of
	// the expected one into the step below, so the check allows a margin on
	// both sides instead, narrow enough that the ranges of the three expected
	// values do not overlap.
	for name, d := range map[string]struct {
		expect, got time.Duration
	}{
		"parent": {40 * durationTestUnit, parent.duration},
		"child1": {30 * durationTestUnit, child1.getDuration()},
		"child2": {20 * durationTestUnit, child2.getDuration()},
	} {
		if d.got < d.expect-4*durationTestUnit || d.got >= d.expect+5*durationTestUnit {
			t.Errorf("%s: expected %s but got %s", name, d.expect, d.got)
		}
	}
}
