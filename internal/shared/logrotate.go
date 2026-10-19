// A tiny dependency-free size-rotating log writer so the log file
// can never grow without bound.

package shared

import (
	"fmt"
	"os"
	"sync"
)

// RotatingWriter writes to a file, rotating to numbered backups (.1, .2, ...)
// once it would exceed maxBytes, keeping at most maxFiles backups.
type RotatingWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	maxFiles int
	f        *os.File
	size     int64
	closed   bool
}

// NewRotatingWriter opens path for appending and returns a writer that rotates it
// at maxBytes, keeping maxFiles numbered backups.
func NewRotatingWriter(path string, maxBytes int64, maxFiles int) (*RotatingWriter, error) {
	w := &RotatingWriter{path: path, maxBytes: maxBytes, maxFiles: maxFiles}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *RotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	w.f = f
	w.size = 0
	if info, err := f.Stat(); err == nil {
		w.size = info.Size()
	}
	return nil
}

// Write appends p, rotating first when it would not fit. A file that could not be
// reopened after a rotation is opened again here, so the writer never keeps a
// closed file and fails silently for ever.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if w.f != nil && w.size+int64(len(p)) > w.maxBytes {
		w.rotate()
	}
	if w.f == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// Close closes the active file.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	if w.f != nil {
		err := w.f.Close()
		w.f = nil
		return err
	}
	return nil
}

// rotate shifts the numbered backups up and starts a fresh active file. When the
// fresh file cannot be opened the writer is left with no file, and the next Write
// tries again.
func (w *RotatingWriter) rotate() {
	w.f.Close()
	w.f = nil
	// Drop the oldest, shift the rest up, then move the active file to .1.
	os.Remove(fmt.Sprintf("%s.%d", w.path, w.maxFiles))
	for i := w.maxFiles - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", w.path, i), fmt.Sprintf("%s.%d", w.path, i+1))
	}
	os.Rename(w.path, w.path+".1")
	w.open() // reopen a fresh, empty active file
}
