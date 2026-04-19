package debug

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	writer  io.Writer
	enabled bool
)

func Init() {
	if os.Getenv("RDTUI_DEBUG") == "" {
		return
	}
	enabled = true
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "rdtui")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "debug.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug: cannot open %s: %v\n", path, err)
		return
	}
	writer = f
	Log("debug log started: %s", path)
}

func Enabled() bool { return enabled }

func Log(format string, args ...interface{}) {
	if !enabled || writer == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	ts := time.Now().Format("15:04:05.000")
	fmt.Fprintf(writer, "[%s] %s\n", ts, fmt.Sprintf(format, args...))
}
