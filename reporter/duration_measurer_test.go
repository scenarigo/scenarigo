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
		time.Sleep(200 * time.Millisecond)
		child1.stop()
		wg.Done()
	}()

	// child1-2
	wg.Add(1)
	go func() {
		<-ch
		time.Sleep(100 * time.Millisecond)
		child1.start()
		time.Sleep(200 * time.Millisecond)
		child1.stop()
		wg.Done()
	}()

	// child2-1
	wg.Add(1)
	go func() {
		<-ch
		time.Sleep(100 * time.Millisecond)
		child2.start()
		time.Sleep(100 * time.Millisecond)
		child2.stop()
		wg.Done()
	}()

	// child2-2
	wg.Add(1)
	go func() {
		<-ch
		time.Sleep(400 * time.Millisecond)
		child2.start()
		time.Sleep(100 * time.Millisecond)
		child2.stop()
		wg.Done()
	}()

	close(ch)
	wg.Wait()

	if expect, got := 400*time.Millisecond, parent.duration.Truncate(10*time.Millisecond); got != expect {
		t.Errorf("expected %s but got %s", expect, got)
	}
	if expect, got := 300*time.Millisecond, child1.duration.Truncate(10*time.Millisecond); got != expect {
		t.Errorf("expected %s but got %s", expect, got)
	}
	if expect, got := 200*time.Millisecond, child2.duration.Truncate(10*time.Millisecond); got != expect {
		t.Errorf("expected %s but got %s", expect, got)
	}
}
