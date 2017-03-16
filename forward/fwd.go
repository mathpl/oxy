// package forwarder implements http handler that forwards requests to remote server
// and serves back the response
// websocket proxying support based on https://github.com/yhat/wsutil
package forward

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/gommon/log"
	"github.com/mathpl/go-tsdmetrics"
	"github.com/rcrowley/go-metrics"
	"github.com/vulcand/oxy/utils"
)

// ReqRewriter can alter request headers and body
type ReqRewriter interface {
	Rewrite(r *http.Request)
}

type optSetter func(f *Forwarder) error

// PassHostHeader specifies if a client's Host header field should
// be delegated
func PassHostHeader(b bool) optSetter {
	return func(f *Forwarder) error {
		f.passHost = b
		return nil
	}
}

// Add counters to track read/written bytes
func Meters(registry tsdmetrics.TaggedRegistry, tags tsdmetrics.Tags) optSetter {
	return func(f *Forwarder) error {
		f.metrics = NewMetricsContext(registry, tags)
		return nil
	}
}

// StreamResponse forces streaming body (flushes response directly to client)
func StreamResponse(b bool) optSetter {
	return func(f *Forwarder) error {
		f.httpForwarder.streamResponse = b
		return nil
	}
}

// RoundTripper sets a new http.RoundTripper
// Forwarder will use http.DefaultTransport as a default round tripper
func RoundTripper(r http.RoundTripper) optSetter {
	return func(f *Forwarder) error {
		f.roundTripper = r
		return nil
	}
}

// Dialer mirrors the net.Dial function to be able to define alternate
// implementations
type Dialer func(network, address string) (net.Conn, error)

// WebsocketDial defines a new network dialer to use to dial to remote websocket destination.
// If no dialer has been defined, net.Dial will be used.
func WebsocketDial(dial Dialer) optSetter {
	return func(f *Forwarder) error {
		f.websocketForwarder.dial = dial
		return nil
	}
}

// Rewriter defines a request rewriter for the HTTP forwarder
func Rewriter(r ReqRewriter) optSetter {
	return func(f *Forwarder) error {
		f.httpForwarder.rewriter = r
		return nil
	}
}

// WebsocketRewriter defines a request rewriter for the websocket forwarder
func WebsocketRewriter(r ReqRewriter) optSetter {
	return func(f *Forwarder) error {
		f.websocketForwarder.rewriter = r
		return nil
	}
}

// ErrorHandler is a functional argument that sets error handler of the server
func ErrorHandler(h utils.ErrorHandler) optSetter {
	return func(f *Forwarder) error {
		f.errHandler = h
		return nil
	}
}

// Logger specifies the logger to use.
// Forwarder will default to oxyutils.NullLogger if no logger has been specified
func Logger(l utils.Logger) optSetter {
	return func(f *Forwarder) error {
		f.log = l
		return nil
	}
}

// Forwarder wraps two traffic forwarding implementations: HTTP and websockets.
// It decides based on the specified request which implementation to use
type Forwarder struct {
	*httpForwarder
	*websocketForwarder
	*handlerContext
}

// handlerContext defines a handler context for error reporting and logging
type handlerContext struct {
	errHandler utils.ErrorHandler
	log        utils.Logger
	bufferPool *sync.Pool

	metrics *metricsContext
}

// httpForwarder is a handler that can reverse proxy
// HTTP traffic
type httpForwarder struct {
	roundTripper   http.RoundTripper
	rewriter       ReqRewriter
	passHost       bool
	streamResponse bool
}

// websocketForwarder is a handler that can reverse proxy
// websocket traffic
type websocketForwarder struct {
	dial            Dialer
	rewriter        ReqRewriter
	TLSClientConfig *tls.Config
}

// New creates an instance of Forwarder based on the provided list of configuration options
func New(setters ...optSetter) (*Forwarder, error) {
	f := &Forwarder{
		httpForwarder:      &httpForwarder{},
		websocketForwarder: &websocketForwarder{},
		handlerContext:     &handlerContext{},
	}
	for _, s := range setters {
		if err := s(f); err != nil {
			return nil, err
		}
	}
	f.bufferPool = &sync.Pool{
		New: func() interface{} {
			return make([]byte, 32*1024)
		},
	}
	if f.httpForwarder.roundTripper == nil {
		f.httpForwarder.roundTripper = http.DefaultTransport
	}
	if f.websocketForwarder.dial == nil {
		f.websocketForwarder.dial = net.Dial
	}
	if f.httpForwarder.rewriter == nil {
		h, err := os.Hostname()
		if err != nil {
			h = "localhost"
		}
		f.httpForwarder.rewriter = &HeaderRewriter{TrustForwardHeader: true, Hostname: h}
	}
	if f.log == nil {
		f.log = utils.NullLogger
	}
	if f.errHandler == nil {
		f.errHandler = utils.DefaultHandler
	}
	return f, nil
}

// ServeHTTP decides which forwarder to use based on the specified
// request and delegates to the proper implementation
func (f *Forwarder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if isWebsocketRequest(req) {
		f.websocketForwarder.serveHTTP(w, req, f.handlerContext)
	} else {
		f.httpForwarder.serveHTTP(w, req, f.handlerContext)
	}
}

// serveHTTP forwards HTTP traffic using the configured transport
func (f *httpForwarder) serveHTTP(w http.ResponseWriter, req *http.Request, ctx *handlerContext) {
	ctx.metrics.httpInit()

	start := time.Now().UTC()

	ctx.metrics.httpRead.Inc(req.ContentLength)
	ctx.metrics.httpConnectionCounter.Inc(1)
	ctx.metrics.httpConnectionOpen.Inc(1)
	defer ctx.metrics.httpConnectionOpen.Dec(1)

	rcr := NewReponseCodeRecorder(w)

	response, err := f.roundTripper.RoundTrip(f.copyRequest(req, req.URL))
	if err != nil {
		ctx.log.Errorf("Error forwarding to %v, err: %v", req.URL, err)
		ctx.errHandler.ServeHTTP(rcr, req, err)
		return
	}

	utils.CopyHeaders(rcr.Header(), response.Header)
	// Remove hop-by-hop headers.
	utils.RemoveHeaders(rcr.Header(), HopHeaders...)
	rcr.WriteHeader(response.StatusCode)

	stream := f.streamResponse
	if !stream {
		contentType, err := utils.GetHeaderMediaType(response.Header, ContentType)
		if err == nil {
			stream = contentType == "text/event-stream"
		}
	}
	//written, err := io.Copy(newResponseFlusher(w, stream), response.Body)
	buffer := ctx.bufferPool.Get().([]byte)
	written, err := io.CopyBuffer(newResponseFlusher(rcr, stream), response.Body, buffer)
	ctx.bufferPool.Put(buffer)
	ctx.metrics.httpWritten.Inc(written)

	if req.TLS != nil {
		ctx.log.Infof("Round trip: %v, code: %v, duration: %v tls:version: %x, tls:resume:%t, tls:csuite:%x, tls:server:%v",
			req.URL, response.StatusCode, time.Now().UTC().Sub(start),
			req.TLS.Version,
			req.TLS.DidResume,
			req.TLS.CipherSuite,
			req.TLS.ServerName)
	} else {
		ctx.log.Infof("Round trip: %v, code: %v, duration: %v",
			req.URL, response.StatusCode, time.Now().UTC().Sub(start))
	}
	defer response.Body.Close()

	defer ctx.metrics.httpResponseTime.Update(time.Now().Sub(start).Nanoseconds())
	defer ctx.metrics.timeWindowedHttpResponseTime.Update(time.Now().Sub(start).Nanoseconds())
	defer ctx.metrics.IncHttpReturnCode(rcr.Code)

	if err != nil {
		ctx.log.Errorf("Error copying upstream response Body: %v", err)
		// Can't write error header at this point
		//ctx.errHandler.ServeHTTP(w, req, err)
		return
	}

	if written != 0 {
		rcr.Header().Set(ContentLength, strconv.FormatInt(written, 10))
	}
}

// copyRequest makes a copy of the specified request to be sent using the configured
// transport
func (f *httpForwarder) copyRequest(req *http.Request, u *url.URL) *http.Request {
	outReq := new(http.Request)
	*outReq = *req // includes shallow copies of maps, but we handle this below

	outReq.URL = utils.CopyURL(req.URL)
	outReq.URL.Scheme = u.Scheme
	outReq.URL.Host = u.Host
	outReq.URL.Path = u.Path
	outReq.URL.RawPath = u.RawPath
	outReq.URL.RawQuery = u.RawQuery
	outReq.URL.Opaque = ""
	// Do not pass client Host header unless optsetter PassHostHeader is set.
	if !f.passHost {
		outReq.Host = u.Host
	}
	outReq.Proto = "HTTP/1.1"
	outReq.ProtoMajor = 1
	outReq.ProtoMinor = 1

	// Overwrite close flag so we can keep persistent connection for the backend servers
	outReq.Close = false

	outReq.Header = make(http.Header)
	utils.CopyHeaders(outReq.Header, req.Header)

	if f.rewriter != nil {
		f.rewriter.Rewrite(outReq)
	}
	return outReq
}

// serveHTTP forwards websocket traffic
func (f *websocketForwarder) serveHTTP(w http.ResponseWriter, req *http.Request, ctx *handlerContext) {
	ctx.metrics.wsInit()

	ctx.metrics.wsConnectionCounter.Inc(1)

	outReq := f.copyRequest(req)
	host := outReq.URL.Host

	// if host does not specify a port, use the default http port
	if !strings.Contains(host, ":") {
		if outReq.URL.Scheme == "wss" {
			host = host + ":443"
		} else {
			host = host + ":80"
		}
	}

	targetConn, err := f.dial("tcp", host)
	if err != nil {
		ctx.log.Errorf("Error dialing `%v`: %v", host, err)
		ctx.errHandler.ServeHTTP(w, req, err)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		ctx.log.Errorf("Unable to hijack the connection: does not implement http.Hijacker")
		ctx.errHandler.ServeHTTP(w, req, err)
		return
	}
	underlyingConn, _, err := hijacker.Hijack()
	if err != nil {
		ctx.log.Errorf("Unable to hijack the connection: %v %v", reflect.TypeOf(w), err)
		ctx.errHandler.ServeHTTP(w, req, err)
		targetConn.Close()
		return
	}
	// it is now caller's responsibility to Close the underlying connection

	// write the modified incoming request to the dialed connection
	if err = outReq.Write(targetConn); err != nil {
		ctx.log.Errorf("Unable to copy request to target: %v", err)
		ctx.errHandler.ServeHTTP(w, req, err)
		underlyingConn.Close()
		targetConn.Close()
		return
	}

	var closing int32

	replicate := func(wg *sync.WaitGroup, dst net.Conn, src net.Conn, copied metrics.Counter) {
		defer wg.Done()
		defer dst.Close()
		defer atomic.StoreInt32(&closing, 1)

		id := fmt.Sprintf("%s -> %s", src.RemoteAddr(), dst.RemoteAddr())

		for {
			fastFail := time.Now().Add(1 * time.Second)
			src.SetReadDeadline(time.Now().Add(15 * time.Second))

			n, err := io.Copy(dst, src)
			copied.Inc(n)

			if err == io.EOF {
				log.Infof("Closing websocket %s: %s.", id, err)
				return
			}

			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}

			if err != nil {
				if atomic.LoadInt32(&closing) == 0 {
					log.Warnf("Closing websocket %s: %s", id, err)
				} else {
					log.Debugf("Closing websocket %s: %s", id, err)
				}
				return
			}

			if n == 0 && time.Now().Before(fastFail) {
				log.Infof("Closing websocket %s: clean close.", id)
				return
			}
		}
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go replicate(wg, targetConn, underlyingConn, ctx.metrics.wsRead)
	go replicate(wg, underlyingConn, targetConn, ctx.metrics.wsWritten)

	ctx.metrics.wsConnectionOpen.Inc(1)
	go func(wg *sync.WaitGroup) {
		wg.Wait()
		ctx.metrics.wsConnectionOpen.Dec(1)
		log.Infof("Closed both side of websocket between %s <-> %s", targetConn.RemoteAddr(), underlyingConn.RemoteAddr())
	}(wg)
}

// copyRequest makes a copy of the specified request.
func (f *websocketForwarder) copyRequest(req *http.Request) (outReq *http.Request) {
	u := req.URL
	outReq = new(http.Request)
	*outReq = *req
	outReq.URL = utils.CopyURL(req.URL)
	outReq.URL.Scheme = req.URL.Scheme
	outReq.URL.Host = req.URL.Host
	outReq.URL.Path = u.Path
	outReq.URL.RawPath = u.RawPath
	outReq.URL.RawQuery = u.RawQuery
	outReq.URL.Opaque = ""

	outReq.Proto = "HTTP/1.1"
	outReq.ProtoMajor = 1
	outReq.ProtoMinor = 1

	// Overwrite close flag so we can keep persistent connection for the backend servers
	outReq.Close = false

	outReq.Header = make(http.Header)
	utils.CopyHeaders(outReq.Header, req.Header)

	if f.rewriter != nil {
		f.rewriter.Rewrite(outReq)
	}
	return outReq
}

// isWebsocketRequest determines if the specified HTTP request is a
// websocket handshake request
func isWebsocketRequest(req *http.Request) bool {
	containsHeader := func(name, value string) bool {
		items := strings.Split(req.Header.Get(name), ",")
		for _, item := range items {
			if value == strings.ToLower(strings.TrimSpace(item)) {
				return true
			}
		}
		return false
	}
	return containsHeader(Connection, "upgrade") && containsHeader(Upgrade, "websocket")
}
