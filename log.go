package middleware

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

type badWriter struct{}

func (_ badWriter) Write(p []byte) (int, error) { return 0, fmt.Errorf("no write set") }
func (_ badWriter) Close() error                { return fmt.Errorf("no write set") }

// LogFile is a persistent file.
type LogFile struct {
	path string // the path to the file
	mode os.FileMode
	mu   *sync.Mutex
	file io.WriteCloser
}

func NewLogFile(path string, mode os.FileMode) (*LogFile, error) {
	lf := &LogFile{path, mode, &sync.Mutex{}, badWriter{}}
	if err := lf.Create(); err != nil {
		return nil, fmt.Errorf("could not create log file: %v", err)
	}
	return lf, nil
}

type nopCloser struct {
	io.Writer
}

var Foo = "bar"

func (nopCloser) Close() error { return nil }

func (lf *LogFile) Close() (err error) {
	lf.mu.Lock()
	err = lf.file.Close()
	lf.file = nopCloser{ioutil.Discard}
	lf.mu.Unlock()
	return
}

func (lf *LogFile) Write(p []byte) (n int, err error) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	n, err = lf.file.Write(p)
	if err != nil {
		if err := lf.Create(); err != nil {
			log.Printf("MIDDLEWARE: could not create log file: %v", err)
		}
	}
	n, err = lf.file.Write(p)
	return
}

func (lf *LogFile) Create() error {
	flags := os.O_WRONLY | os.O_APPEND | os.O_CREATE
	file, err := os.OpenFile(lf.path, flags, lf.mode)
	if err != nil {
		lf.file = badWriter{}
	} else {
		lf.file = file
	}
	return err
}

const defaultStatus int = -1

type statusWrapper struct {
	wr     http.ResponseWriter
	status int
	nbytes int
}

func (w *statusWrapper) Header() http.Header { return w.wr.Header() }

func (w *statusWrapper) WriteHeader(status int) {
	w.wr.WriteHeader(status)
	w.status = status
}

func (w *statusWrapper) Write(p []byte) (n int, err error) {
	if w.status == defaultStatus {
		w.status = http.StatusOK
	}
	n, err = w.wr.Write(p)
	w.nbytes += n
	return
}

type statusHijacker struct {
	*statusWrapper
	hijacker http.Hijacker
}

func (w *statusHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	// if the ResponseWriter is hijacked we'll assume the request was a success
	if w.statusWrapper.status == defaultStatus {
		w.statusWrapper.status = http.StatusOK
	}
	return w.hijacker.Hijack()
}

func Log(log io.Writer, h http.Handler) http.Handler {
	hf := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		path := r.URL.String()
		method := r.Method
		origin := r.Header.Get("X-Forwarded-For")
		if origin == "" {
			origin = r.RemoteAddr
		}
		userAgent := r.UserAgent()
		wrapper := &statusWrapper{w, defaultStatus, 0}
		hijacker, ok := w.(http.Hijacker)
		if ok {
			w = &statusHijacker{wrapper, hijacker}
		} else {
			w = wrapper
		}

		h.ServeHTTP(w, r)

		diff := time.Since(start)
		args := []interface{}{
			start.Format("2006/01/02 15:04:05"),
			method,
			wrapper.status,
			path,
			origin,
			wrapper.nbytes,
			diff.String(),
			userAgent,
		}
		fmt.Fprintf(log, "%s %s %d %s %s %d %s '%s'\n", args...)
	}
	return http.HandlerFunc(hf)
}
