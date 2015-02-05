// Middleware for the net/http library
package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"
)

type gzipWrapper struct {
	w   http.ResponseWriter
	gzw *gzip.Writer // always wraps the ResponseWriter
}

func (wrapper *gzipWrapper) Header() http.Header {
	return wrapper.w.Header()
}

func (wrapper *gzipWrapper) Write(p []byte) (int, error) {
	// this is the same thing the default ResponseWriter does
	if "" == wrapper.Header().Get("Content-Type") {
		wrapper.Header().Set("Content-Type", http.DetectContentType(p))
	}
	return wrapper.gzw.Write(p)
}

func (wrapper *gzipWrapper) WriteHeader(status int) {
	wrapper.w.WriteHeader(status)
}

// GZip returns a handler which gzip's the response for all requests which
// accept that encoding. A gzip'ed ResponseWriter does not implement the
// http.Hijacker interface.
func GZip(h http.Handler) http.Handler {
	hfunc := func(w http.ResponseWriter, r *http.Request) {
		ae := strings.Split(r.Header.Get("Accept-Encoding"), ",")
		acceptsGzip := false
		for i := range ae {
			if strings.TrimSpace(ae[i]) == "gzip" {
				acceptsGzip = true
				break
			}
		}
		if acceptsGzip {
			w.Header().Set("Content-Encoding", "gzip")
			gzw := gzip.NewWriter(w)
			defer gzw.Close()
			w = &gzipWrapper{w, gzw}
		}
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(hfunc)
}
