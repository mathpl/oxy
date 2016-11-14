package forward

import (
	"log"
	"sync"

	"github.com/mathpl/go-tsdmetrics"
	"github.com/rcrowley/go-metrics"
)

type metricsContext struct {
	registry tsdmetrics.TaggedRegistry
	tags     tsdmetrics.Tags

	initMutex              sync.Mutex
	httpStarted, wsStarted bool

	httpRead,
	httpWritten,
	httpConnectionCounter,
	httpConnectionOpen metrics.Counter

	httpResponseTime tsdmetrics.IntegerHistogram
	httpReturnCode100,
	httpReturnCode200,
	httpReturnCode300,
	httpReturnCode400,
	httpReturnCode500 metrics.Counter

	wsRead,
	wsWritten,
	wsConnectionCounter,
	wsConnectionOpen metrics.Counter
	wsSessionTime tsdmetrics.IntegerHistogram
}

func NewMetricsContext(registry tsdmetrics.TaggedRegistry, tags tsdmetrics.Tags) *metricsContext {
	return &metricsContext{
		registry: registry,
		tags:     tags,
	}
}

func (ctx *metricsContext) httpInit() {
	ctx.initMutex.Lock()
	defer ctx.initMutex.Unlock()

	if ctx.httpStarted {
		return
	}

	ctx.httpStarted = true

	httpTags := ctx.tags.AddTags(tsdmetrics.Tags{"conn_type": "http"})
	newRead := metrics.NewCounter()
	read, ok := ctx.registry.GetOrRegister("bytes", httpTags.AddTags(tsdmetrics.Tags{"direction": "in"}), newRead).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: bytes %s", ctx.tags)
	}
	ctx.httpRead = read

	newWritten := metrics.NewCounter()
	written, ok := ctx.registry.GetOrRegister("bytes", httpTags.AddTags(tsdmetrics.Tags{"direction": "out"}), newWritten).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: bytes %s", ctx.tags)
	}
	ctx.httpWritten = written

	newHisto := tsdmetrics.NewIntegerHistogram(metrics.NewExpDecaySample(512, 0.15))
	histo, ok := ctx.registry.GetOrRegister("response.time.ns", httpTags, newHisto).(tsdmetrics.IntegerHistogram)
	if !ok {
		log.Fatalf("Invalid type registered for: response.time.ns %s", httpTags)
	}
	ctx.httpResponseTime = histo

	newConnectionCounter := metrics.NewCounter()
	count, ok := ctx.registry.GetOrRegister("connection.count", httpTags, newConnectionCounter).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: connection.count %s", ctx.tags)
	}
	ctx.httpConnectionCounter = count

	newConnectionOpen := metrics.NewCounter()
	open, ok := ctx.registry.GetOrRegister("connection.open", httpTags, newConnectionOpen).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: connection.open %s", ctx.tags)
	}
	ctx.httpConnectionOpen = open

	// Each http codes
	newHttpReturnCode100 := metrics.NewCounter()
	c, ok := ctx.registry.GetOrRegister("response.count", tsdmetrics.Tags{"httpcode": "1xx"}, newHttpReturnCode100).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: response.count %s", ctx.tags)
	}
	ctx.httpReturnCode100 = c

	newHttpReturnCode200 := metrics.NewCounter()
	c, ok = ctx.registry.GetOrRegister("response.count", tsdmetrics.Tags{"httpcode": "2xx"}, newHttpReturnCode200).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: response.count %s", ctx.tags)
	}
	ctx.httpReturnCode200 = c

	newHttpReturnCode300 := metrics.NewCounter()
	c, ok = ctx.registry.GetOrRegister("response.count", tsdmetrics.Tags{"httpcode": "3xx"}, newHttpReturnCode300).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: response.count %s", ctx.tags)
	}
	ctx.httpReturnCode300 = c

	newHttpReturnCode400 := metrics.NewCounter()
	c, ok = ctx.registry.GetOrRegister("response.count", tsdmetrics.Tags{"httpcode": "4xx"}, newHttpReturnCode400).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: response.count %s", ctx.tags)
	}
	ctx.httpReturnCode400 = c

	newHttpReturnCode500 := metrics.NewCounter()
	c, ok = ctx.registry.GetOrRegister("response.count", tsdmetrics.Tags{"httpcode": "5xx"}, newHttpReturnCode500).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: response.count %s", ctx.tags)
	}
	ctx.httpReturnCode500 = c

}

func (ctx *metricsContext) wsInit() {
	ctx.initMutex.Lock()
	defer ctx.initMutex.Unlock()

	if ctx.wsStarted {
		return
	}

	ctx.wsStarted = true

	wsTags := ctx.tags.AddTags(tsdmetrics.Tags{"conn_type": "websocket"})
	newRead := metrics.NewCounter()
	read, ok := ctx.registry.GetOrRegister("bytes", wsTags.AddTags(tsdmetrics.Tags{"direction": "in"}), newRead).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: bytes %s", ctx.tags)
	}
	ctx.wsRead = read

	newWritten := metrics.NewCounter()
	written, ok := ctx.registry.GetOrRegister("bytes", wsTags.AddTags(tsdmetrics.Tags{"direction": "out"}), newWritten).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: bytes %s", ctx.tags)
	}
	ctx.wsWritten = written

	newHisto := tsdmetrics.NewIntegerHistogram(metrics.NewExpDecaySample(512, 0.15))
	histo, ok := ctx.registry.GetOrRegister("session.time.ns", wsTags, newHisto).(tsdmetrics.IntegerHistogram)
	if !ok {
		log.Fatalf("Invalid type registered for: response.time.ns %s", wsTags)
	}
	ctx.wsSessionTime = histo

	newConnectionCounter := metrics.NewCounter()
	count, ok := ctx.registry.GetOrRegister("connection.count", wsTags, newConnectionCounter).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: connection.count %s", ctx.tags)
	}
	ctx.wsConnectionCounter = count

	newConnectionOpen := metrics.NewCounter()
	open, ok := ctx.registry.GetOrRegister("connection.open", wsTags, newConnectionOpen).(metrics.Counter)
	if !ok {
		log.Fatalf("Invalid type registered for: connection.open %s", ctx.tags)
	}
	ctx.wsConnectionOpen = open
}

func (ctx *metricsContext) IncHttpReturnCode(code int) {
	highCode := uint8(code / 100)

	var c metrics.Counter
	switch highCode {
	case 1:
		c = ctx.httpReturnCode100
	case 2:
		c = ctx.httpReturnCode200
	case 3:
		c = ctx.httpReturnCode300
	case 4:
		c = ctx.httpReturnCode400
	case 5:
		c = ctx.httpReturnCode500
	default:
		// Unexpected http return code, ignore
		return
	}

	c.Inc(1)
}
