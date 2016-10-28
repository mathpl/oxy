package forward

import (
	"fmt"
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
	httpReturnCode   map[uint8]metrics.Counter

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

	if ctx.httpReturnCode == nil {
		ctx.httpReturnCode = make(map[uint8]metrics.Counter, 5)
	}
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

	ctx.initMutex.Lock()
	defer ctx.initMutex.Unlock()

	c, found := ctx.httpReturnCode[highCode]

	if !found {
		newC := metrics.NewCounter()
		ctx.httpReturnCode[highCode] = newC

		tags := ctx.tags.AddTags(tsdmetrics.Tags{"conn_type": "http", "httpcode": fmt.Sprintf("%dxx", highCode)})

		var ok bool
		c, ok = ctx.registry.GetOrRegister("response.count", tags, newC).(metrics.Counter)

		if !ok {
			log.Fatalf("Invalid type registered for: response.count %s", tags)
		}
	}
	c.Inc(1)
}
