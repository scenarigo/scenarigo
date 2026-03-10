package reporter

import (
	"regexp"
	"strings"
)

type matcher struct {
	patterns []*regexp.Regexp
}

func newMatcher(run string) (*matcher, error) {
	if run == "" {
		return nil, nil //nolint: nilnil
	}

	exprs := strings.Split(run, "/")
	m := &matcher{}
	for _, e := range exprs {
		p, err := regexp.Compile(e)
		if err != nil {
			return nil, err
		}
		m.patterns = append(m.patterns, p)
	}
	return m, nil
}

func (m *matcher) match(parent, name string) bool {
	if m == nil {
		return true
	}
	depth := strings.Count(parent, "/") + 1
	for s := range strings.SplitSeq(name, "/") {
		if depth >= len(m.patterns) {
			return true
		}
		if !m.patterns[depth].MatchString(s) {
			return false
		}
		depth++
	}
	return true
}
