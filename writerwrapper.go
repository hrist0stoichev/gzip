package gzip

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
)

// writerWrapper wraps the originalHandler
// to test whether to gzip and gzip the body if applicable.
type writerWrapper struct {
	// header filter are applied by its sequence
	Filters []ResponseHeaderFilter
	// min content length to enable compress
	MinContentLength int64
	OriginWriter     http.ResponseWriter
	// use initGzipWriter() to init gzipWriter when in need
	GetGzipWriter func() *gzip.Writer
	// must close gzip writer and put it back to pool
	PutGzipWriter func(*gzip.Writer)

	// internal below

	// compress or not
	// default to true
	shouldCompress bool
	gzipWriter     *gzip.Writer
	// is header already flushed?
	headerFlushed bool
	didFirstWrite bool
	statusCode    int
}

func newWriterWrapper(filters []ResponseHeaderFilter, minContentLength int64, originWriter http.ResponseWriter, getGzipWriter func() *gzip.Writer, putGzipWriter func(*gzip.Writer)) *writerWrapper {
	return &writerWrapper{
		shouldCompress:   true,
		Filters:          filters,
		MinContentLength: minContentLength,
		OriginWriter:     originWriter,
		GetGzipWriter:    getGzipWriter,
		PutGzipWriter:    putGzipWriter,
	}
}

// interface guard
var _ http.ResponseWriter = (*writerWrapper)(nil)
var _ http.Flusher = (*writerWrapper)(nil)

func (w *writerWrapper) headerWritten() bool {
	return w.statusCode != 0
}

func (w *writerWrapper) initGzipWriter() {
	w.gzipWriter = w.GetGzipWriter()
	w.gzipWriter.Reset(w.OriginWriter)
}

// Header implements http.ResponseWriter
func (w *writerWrapper) Header() http.Header {
	return w.OriginWriter.Header()
}

// Write implements http.ResponseWriter
func (w *writerWrapper) Write(data []byte) (int, error) {
	if !w.headerWritten() {
		w.WriteHeader(http.StatusOK)
	}

	header := w.Header()
	for _, filter := range w.Filters {
		w.shouldCompress = filter.ShouldCompress(header)
		if !w.shouldCompress {
			break
		}
	}

	// use origin handler directly
	if !w.shouldCompress {
		w.flushHeader()
		w.didFirstWrite = true
		return w.OriginWriter.Write(data)
	}

	if !w.didFirstWrite {
		w.didFirstWrite = true
		w.shouldCompress = w.enoughContentLength(data)
		w.flushHeader()

		if w.shouldCompress {
			w.initGzipWriter()
		}
	}

	if w.shouldCompress {
		return w.gzipWriter.Write(data)
	}

	return w.OriginWriter.Write(data)
}

func (w *writerWrapper) enoughContentLength(data []byte) bool {
	var (
		header        = w.Header()
		_, haveCl     = header["Content-Length"]
		contentLength int64
	)
	if haveCl {
		contentLength, _ = strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	} else {
		contentLength = int64(len(data))
	}

	if contentLength == 0 || contentLength < w.MinContentLength {
		return false
	}

	return true
}

// WriteHeader implements http.ResponseWriter
//
// WriteHeader does not really calls originalHandler's WriteHeader,
// and the calling will actually be handler by flushHeader().
func (w *writerWrapper) WriteHeader(statusCode int) {
	if w.headerWritten() {
		return
	}

	w.statusCode = statusCode

	if !w.shouldCompress {
		return
	}

	if statusCode == http.StatusNoContent ||
		statusCode == http.StatusNotModified {
		w.shouldCompress = false
		return
	}
}

// flushHeader must always be called and called after
// WriteHeader() is called and
// w.shouldCompress is decided.
func (w *writerWrapper) flushHeader() {
	if w.headerFlushed {
		return
	}

	// if neither WriteHeader() or Write() are called,
	// do nothing
	if !w.headerWritten() {
		return
	}

	if w.shouldCompress {
		header := w.Header()
		header.Del("Content-Length")
		header.Set("Content-Encoding", "gzip")
		header.Add("Vary", "Accept-Encoding")
		originalEtag := w.Header().Get("ETag")
		if originalEtag != "" && !strings.HasPrefix(originalEtag, "W/") {
			w.Header().Set("ETag", "W/"+originalEtag)
		}
	}

	w.OriginWriter.WriteHeader(w.statusCode)

	w.headerFlushed = true
}

// CleanUp flushes header and closed gzip writer
//
// Write() and WriteHeader() should not be called
// after CleanUp()
func (w *writerWrapper) CleanUp() {
	w.flushHeader()
	if w.gzipWriter != nil {
		w.PutGzipWriter(w.gzipWriter)
		w.gzipWriter = nil
	}
}

// Flush implements http.Flusher
func (w *writerWrapper) Flush() {
	w.CleanUp()

	if flusher, ok := w.OriginWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}