package guard

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Logger is the guard's line-oriented timestamped logger.
type Logger struct {
	mu  sync.Mutex
	out io.Writer
}

// NewLogger creates a new guard logger.
func NewLogger(out io.Writer) *Logger {
	return &Logger{out: out}
}

// Printf writes one timestamped line.
func (l *Logger) Printf(format string, args ...any) {
	if l == nil || l.out == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(l.out, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}
