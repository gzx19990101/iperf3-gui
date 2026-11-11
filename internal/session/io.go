package session

import (
	"bytes"
	"strings"
	"sync"
)

type streamWriter struct {
	mu     sync.Mutex
	buf    []byte
	onLine func([]byte)
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		index := bytes.IndexByte(w.buf, '\n')
		if index < 0 {
			break
		}
		line := append([]byte(nil), w.buf[:index]...)
		line = bytes.TrimSuffix(line, []byte("\r"))
		w.buf = w.buf[index+1:]
		w.onLine(line)
	}
	return len(p), nil
}

func (w *streamWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return
	}
	line := append([]byte(nil), w.buf...)
	w.buf = nil
	w.onLine(line)
}

type boundedBufferWriter struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func newBoundedBufferWriter(limit int) *boundedBufferWriter {
	return &boundedBufferWriter{limit: limit}
}

func (w *boundedBufferWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	remaining := w.limit - w.buffer.Len()
	if remaining <= 0 {
		w.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = w.buffer.Write(p[:remaining])
		w.truncated = true
	} else {
		_, _ = w.buffer.Write(p)
	}
	return len(p), nil
}

func (w *boundedBufferWriter) String() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String(), w.truncated
}

func appendBounded(builder *strings.Builder, text string, limit int) bool {
	if builder.Len() >= limit {
		return true
	}
	remaining := limit - builder.Len()
	if len(text) > remaining {
		builder.WriteString(text[:remaining])
		return true
	}
	builder.WriteString(text)
	return false
}
