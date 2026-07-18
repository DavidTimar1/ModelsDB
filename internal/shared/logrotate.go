// Package shared - a tiny dependency-free size-rotating log writer so the log file
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
}

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

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > w.maxBytes {
		w.rotate()
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f != nil {
		return w.f.Close()
	}
	return nil
}

func (w *RotatingWriter) rotate() {
	w.f.Close()
	// Drop the oldest, shift the rest up, then move the active file to .1.
	os.Remove(fmt.Sprintf("%s.%d", w.path, w.maxFiles))
	for i := w.maxFiles - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", w.path, i), fmt.Sprintf("%s.%d", w.path, i+1))
	}
	os.Rename(w.path, w.path+".1")
	w.open() // reopen a fresh, empty active file
}
