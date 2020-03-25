package parser

// Position describes an arbitrary source position.
type Position struct {
	Line   int
	Column int
	Offset int
}

type posCalculator struct {
	buf   []byte
	lines []int
}

// Write implements io.Writer interface.
func (c *posCalculator) Write(p []byte) (n int, err error) {
	n = len(p)
	c.buf = append(c.buf, p...)
	var last int
	for i, b := range c.buf {
		if b == '\n' {
			line := c.buf[last : i+1]
			c.lines = append(c.lines, len([]rune(string(line))))
			last = i + 1
		}
	}
	c.buf = c.buf[last:]
	return
}

// Pos returns the Position value for the given offset.
func (c *posCalculator) Pos(pos int) *Position {
	line, pre, sum := 1, 0, 0
	for _, l := range c.lines {
		sum += l
		if pos < sum {
			break
		}
		pre = sum
		line++
	}
	return &Position{
		Line:   line,
		Column: pos - pre,
		Offset: pos,
	}
}
