package reporter

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
)

type outputCapturer struct {
	mu        sync.Mutex
	name      string
	target    **os.File
	orig      *os.File
	reader    *os.File
	writer    *os.File
	buf       bytes.Buffer
	done      chan struct{}
	capturing bool
}

func newOutputCapturer(name string, target **os.File) *outputCapturer {
	return &outputCapturer{
		name:   name,
		target: target,
	}
}

var globalStdoutCapturer = newOutputCapturer("stdout", &os.Stdout)

func (c *outputCapturer) start() (func() (string, error), error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.capturing {
		return nil, fmt.Errorf("%s capture already started", c.name)
	}
	if c.target == nil {
		return nil, fmt.Errorf("%s target is nil", c.name)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create %s pipe: %w", c.name, err)
	}
	c.orig = *c.target
	c.reader = reader
	c.writer = writer
	c.buf.Reset()
	c.done = make(chan struct{})
	*c.target = writer
	c.capturing = true
	go func() {
		_, _ = io.Copy(&c.buf, reader)
		close(c.done)
	}()

	return c.stop, nil
}

func (c *outputCapturer) stop() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.capturing {
		return "", fmt.Errorf("%s capture not started", c.name)
	}

	if c.writer != nil {
		_ = c.writer.Close()
	}
	if c.done != nil {
		<-c.done
	}
	if c.reader != nil {
		_ = c.reader.Close()
	}
	if c.target != nil && c.orig != nil {
		*c.target = c.orig
	}
	captured := c.buf.String()
	c.buf.Reset()
	c.orig = nil
	c.reader = nil
	c.writer = nil
	c.done = nil
	c.capturing = false

	return captured, nil
}
