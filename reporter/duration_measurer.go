package reporter

import (
	"sync"
	"time"
)

type durationMeasurer struct {
	parent    *durationMeasurer
	m         sync.Mutex
	running   int
	duration  time.Duration
	startTime time.Time
}

func (m *durationMeasurer) start() {
	if m == nil {
		return
	}
	m.parent.start()
	m.m.Lock()
	defer m.m.Unlock()
	if m.running == 0 {
		m.startTime = time.Now()
	}
	m.running++
}

func (m *durationMeasurer) stop() {
	if m == nil {
		return
	}
	m.parent.stop()
	m.m.Lock()
	defer m.m.Unlock()
	m.running--
	if m.running == 0 {
		m.duration += time.Since(m.startTime)
	}
}

func (m *durationMeasurer) spawn() *durationMeasurer {
	return &durationMeasurer{
		parent: m,
	}
}

func (m *durationMeasurer) getDuration() time.Duration {
	m.m.Lock()
	defer m.m.Unlock()
	return m.duration
}
