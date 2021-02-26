package reporter

import (
	"sync"
	"testing"
	"time"
)

/*
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
	wg.Add(1)
	go func() {
		<-ch
		child1.start()
		time.Sleep(20 * durationTestUnit)
		child1.stop()
		wg.Done()
	}()

	// child1-2
	wg.Add(1)
	go func() {
		<-ch
		time.Sleep(10 * durationTestUnit)
		child1.start()
		time.Sleep(20 * durationTestUnit)
		child1.stop()
		wg.Done()
	}()

	// child2-1
	wg.Add(1)
	go func() {
		<-ch
		time.Sleep(10 * durationTestUnit)
		child2.start()
		time.Sleep(10 * durationTestUnit)
		child2.stop()
		wg.Done()
	}()

	// child2-2
	wg.Add(1)
	go func() {
		<-ch
		time.Sleep(40 * durationTestUnit)
		child2.start()
		time.Sleep(10 * durationTestUnit)
		child2.stop()
		wg.Done()
	}()

	close(ch)
	wg.Wait()

	if expect, got := 40*durationTestUnit, parent.duration.Truncate(durationTestUnit); got != expect {
		t.Errorf("expected %s but got %s", expect, got)
	}
	if expect, got := 30*durationTestUnit, child1.duration.Truncate(durationTestUnit); got != expect {
		t.Errorf("expected %s but got %s", expect, got)
	}
	if expect, got := 20*durationTestUnit, child2.duration.Truncate(durationTestUnit); got != expect {
		t.Errorf("expected %s but got %s", expect, got)
	}
}
