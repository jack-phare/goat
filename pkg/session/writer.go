package session

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

const (
	writerBufferSize = 256
	flushIdleTimeout = 100 * time.Millisecond
)

// writeOp is a single write request for the async writer.
type writeOp struct {
	path string
	data []byte
	err  chan error // optional: nil if caller doesn't need confirmation
}

// asyncWriter batches file writes in a background goroutine.
type asyncWriter struct {
	ch    chan writeOp
	done  chan struct{}
	mu    sync.Mutex
	files map[string]*os.File
}

func newAsyncWriter() *asyncWriter {
	w := &asyncWriter{
		ch:    make(chan writeOp, writerBufferSize),
		done:  make(chan struct{}),
		files: make(map[string]*os.File),
	}
	go w.run()
	return w
}

func (w *asyncWriter) run() {
	defer close(w.done)

	timer := time.NewTimer(flushIdleTimeout)
	defer timer.Stop()

	var pending []writeOp

	for {
		select {
		case op, ok := <-w.ch:
			if !ok {
				// Channel closed — flush remaining and exit
				w.flushAll(pending)
				return
			}
			pending = append(pending, op)

			// Drain any more that are immediately available
			for {
				select {
				case op2, ok2 := <-w.ch:
					if !ok2 {
						w.flushAll(pending)
						return
					}
					pending = append(pending, op2)
				default:
					goto batchDone
				}
			}
		batchDone:
			w.flushAll(pending)
			pending = pending[:0]
			timer.Reset(flushIdleTimeout)

		case <-timer.C:
			// Idle timeout — flush anything pending (usually empty)
			if len(pending) > 0 {
				w.flushAll(pending)
				pending = pending[:0]
			}
			timer.Reset(flushIdleTimeout)
		}
	}
}

func (w *asyncWriter) flushAll(ops []writeOp) {
	for _, op := range ops {
		err := w.writeToFile(op.path, op.data)
		if op.err != nil {
			op.err <- err
		}
	}
}

const lockTimeout = 5 * time.Second

func (w *asyncWriter) writeToFile(path string, data []byte) error {
	w.mu.Lock()
	f, ok := w.files[path]
	if !ok {
		var err error
		f, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			w.mu.Unlock()
			return err
		}
		w.files[path] = f
	}
	w.mu.Unlock()

	// Acquire per-file lock for cross-process safety
	fl := flock.New(path + ".lock")
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()

	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil || !locked {
		return ErrLockTimeout
	}
	defer fl.Unlock()

	_, err = f.Write(data)
	return err
}

// Write enqueues a write operation. If errCh is non-nil, the error is sent on it.
func (w *asyncWriter) Write(path string, data []byte, errCh chan error) {
	w.ch <- writeOp{path: path, data: data, err: errCh}
}

// Close signals the writer to flush and stop, then closes all file handles.
func (w *asyncWriter) Close() error {
	close(w.ch)
	<-w.done // wait for goroutine to finish

	w.mu.Lock()
	defer w.mu.Unlock()

	var lastErr error
	for _, f := range w.files {
		if err := f.Close(); err != nil {
			lastErr = err
		}
	}
	w.files = nil
	return lastErr
}
