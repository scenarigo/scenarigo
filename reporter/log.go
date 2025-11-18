package reporter

import (
	"sort"
	"sync"
)

type logRecorder struct {
	m         sync.Mutex
	strs      []string
	printIdxs []int
	infoIdxs  []int
	errorIdxs []int
	skipIdx   *int
	replacer  LogReplacer
}

func (r *logRecorder) spawn() *logRecorder {
	r.m.Lock()
	defer r.m.Unlock()
	return &logRecorder{
		replacer: r.replacer,
	}
}

func (r *logRecorder) setReplacer(rep LogReplacer) {
	r.m.Lock()
	defer r.m.Unlock()
	r.replacer = rep
	for i, s := range r.strs {
		r.strs[i] = r.replacer.ReplaceAll(s)
	}
}

func (r *logRecorder) print(s string) {
	if r.replacer != nil {
		s = r.replacer.ReplaceAll(s)
	}
	r.m.Lock()
	defer r.m.Unlock()
	r.strs = append(r.strs, s)
	r.printIdxs = append(r.printIdxs, len(r.strs)-1)
}

func (r *logRecorder) log(s string) {
	if r.replacer != nil {
		s = r.replacer.ReplaceAll(s)
	}
	r.m.Lock()
	defer r.m.Unlock()
	r.strs = append(r.strs, s)
	r.infoIdxs = append(r.infoIdxs, len(r.strs)-1)
}

func (r *logRecorder) error(s string) {
	if r.replacer != nil {
		s = r.replacer.ReplaceAll(s)
	}
	r.m.Lock()
	defer r.m.Unlock()
	r.strs = append(r.strs, s)
	r.errorIdxs = append(r.errorIdxs, len(r.strs)-1)
}

func (r *logRecorder) skip(s string) {
	if r.replacer != nil {
		s = r.replacer.ReplaceAll(s)
	}
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
	copy(strs, r.strs)
	return strs
}

func (r *logRecorder) printLogs() []string {
	r.m.Lock()
	defer r.m.Unlock()
	strs := make([]string, len(r.printIdxs))
	for i, j := range r.printIdxs {
		strs[i] = r.strs[j]
	}
	if len(strs) == 0 {
		return nil
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

func (r *logRecorder) append(s *logRecorder) {
	r.m.Lock()
	defer r.m.Unlock()
	s.m.Lock()
	defer s.m.Unlock()
	cc := len(r.strs)
	r.strs = append(r.strs, s.strs...)
	for _, idx := range s.infoIdxs {
		r.infoIdxs = append(r.infoIdxs, idx+cc)
	}
	for _, idx := range s.errorIdxs {
		r.errorIdxs = append(r.errorIdxs, idx+cc)
	}
	for _, idx := range s.printIdxs {
		r.printIdxs = append(r.printIdxs, idx+cc)
	}
	if s.skipIdx != nil {
		if r.skipIdx == nil {
			idx := *s.skipIdx + cc
			r.skipIdx = &idx
		} else {
			r.infoIdxs = append(r.infoIdxs, *s.skipIdx+cc)
			sort.Ints(r.infoIdxs)
		}
	}
}

type LogReplacer interface {
	ReplaceAll(string) string
}
