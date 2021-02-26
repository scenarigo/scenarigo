package reporter

import "sync"

type logRecorder struct {
	m         sync.Mutex
	strs      []string
	infoIdxs  []int
	errorIdxs []int
	skipIdx   *int
}

func (r *logRecorder) log(s string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.strs = append(r.strs, s)
	r.infoIdxs = append(r.infoIdxs, len(r.strs)-1)
}

func (r *logRecorder) error(s string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.strs = append(r.strs, s)
	r.errorIdxs = append(r.errorIdxs, len(r.strs)-1)
}

func (r *logRecorder) skip(s string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.strs = append(r.strs, s)
	i := len(r.strs) - 1
	r.skipIdx = &i
}

func (r *logRecorder) all() []string {
	r.m.Lock()
	defer r.m.Unlock()
	strs := make([]string, len(r.strs))
	for i, str := range r.strs {
		strs[i] = str
	}
	return strs
}

func (r *logRecorder) infoLogs() []string {
	r.m.Lock()
	defer r.m.Unlock()
	ignore := map[int]struct{}{}
	for _, i := range r.errorIdxs {
		ignore[i] = struct{}{}
	}
	if r.skipIdx != nil {
		ignore[*r.skipIdx] = struct{}{}
	}
	strs := make([]string, 0, len(r.strs)-len(ignore))
	for i, str := range r.strs {
		if _, ok := ignore[i]; ok {
			continue
		}
		strs = append(strs, str)
	}
	if len(strs) == 0 {
		return nil
	}
	return strs
}

func (r *logRecorder) errorLogs() []string {
	r.m.Lock()
	defer r.m.Unlock()
	strs := make([]string, len(r.errorIdxs))
	for i, j := range r.errorIdxs {
		strs[i] = r.strs[j]
	}
	if len(strs) == 0 {
		return nil
	}
	return strs
}

func (r *logRecorder) skipLog() *string {
	r.m.Lock()
	defer r.m.Unlock()
	if r.skipIdx == nil {
		return nil
	}
	return &r.strs[*r.skipIdx]
}
