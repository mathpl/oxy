package forward

import (
	"bufio"
	"net"
	"net/http"
)

type ResponseCodeRecorder struct {
	Code int
	rw   http.ResponseWriter
}

func NewReponseCodeRecorder(rw http.ResponseWriter) *ResponseCodeRecorder {
	return &ResponseCodeRecorder{
		Code: 200,
		rw:   rw,
	}
}

func (rcr *ResponseCodeRecorder) Header() http.Header {
	return rcr.rw.Header()
}

func (rcr *ResponseCodeRecorder) Write(buf []byte) (int, error) {
	return rcr.rw.Write(buf)
}

func (rcr *ResponseCodeRecorder) WriteHeader(code int) {
	rcr.Code = code
	rcr.rw.WriteHeader(code)
}

func (rcr *ResponseCodeRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return rcr.rw.(http.Hijacker).Hijack()
}
