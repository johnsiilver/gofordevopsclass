package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"
)

// main sets up the trace and metrics providers and starts a loop to continuously call the server
func main() {
	meterProvider, shutdown := initTraceAndMetricsProvider()
	defer shutdown()

	continuouslySendRequests(meterProvider)
}

// initTraceAndMetricsProvider initializes an OTLP exporter, and configures the corresponding trace and
// metric providers.
func initTraceAndMetricsProvider() (*sdkmetric.MeterProvider, func()) {
	ctx := context.Background()

	otelAgentAddr, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if !ok {
		otelAgentAddr = "0.0.0.0:4317"
	}

	meterProvider, closeMetrics := initMetrics(ctx, otelAgentAddr)
	closeTraces := initTracer(ctx, otelAgentAddr)

	return meterProvider, func() {
		doneCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		// pushes any last exports to the receiver
		closeTraces(doneCtx)
		closeMetrics(doneCtx)
	}
}

// initTracer initializes an OTLP trace exporter and registers the trace provider with the global context
func initTracer(ctx context.Context, otelAgentAddr string) func(context.Context) {
	traceClient := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(otelAgentAddr),
		otlptracegrpc.WithDialOption(grpc.WithBlock()))
	traceExp, err := otlptrace.New(ctx, traceClient)
	handleErr(err, "Failed to create the collector trace exporter")

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String("demo-client"),
		),
	)
	handleErr(err, "failed to create resource")

	bsp := sdktrace.NewBatchSpanProcessor(traceExp)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(tracerProvider)

	return func(doneCtx context.Context) {
		if err := traceExp.Shutdown(doneCtx); err != nil {
			otel.Handle(err)
		}
	}
}

// initMetrics initializes a metrics pusher and registers the metrics provider with the global context
func initMetrics(ctx context.Context, otelAgentAddr string) (*sdkmetric.MeterProvider, func(context.Context)) {
	exp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(otelAgentAddr),
	)
	if err != nil {
		panic(err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service", "demo-client")),
		resource.WithHost(),
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String("demo-client"),
		),
	)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	return meterProvider, func(doneCtx context.Context) {
		// pushes any last exports to the receiver
		if err := meterProvider.Shutdown(ctx); err != nil {
			handleErr(err, "Failed to shutdown the collector metric exporter")
		}
	}
}

// handleErr provides a simple way to handle errors and messages
func handleErr(err error, message string) {
	if err != nil {
		log.Fatalf("%s: %v", message, err)
	}
}

// continuouslySendRequests continuously sends requests to the server and generates random lines of text to be measured
func continuouslySendRequests(meterProvider *sdkmetric.MeterProvider) {
	var (
		tracer       = otel.Tracer("demo-client-tracer")
		meter        = meterProvider.Meter("demo_client", metric.WithInstrumentationVersion("v1.0.0"))
		instruments  = NewClientInstruments(meter)
		commonLabels = []attribute.KeyValue{
			attribute.String("method", "repl"),
			attribute.String("client", "cli"),
		}
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	)

	for {
		startTime := time.Now()
		ctx, span := tracer.Start(context.Background(), "ExecuteRequest")
		makeRequest(ctx)
		span.End()
		latencyMs := float64(time.Since(startTime)) / 1e6
		nr := int(rng.Int31n(7))
		for i := 0; i < nr; i++ {
			randLineLength := rng.Int63n(999)
			instruments.LineCounts.Add(ctx, 1, metric.WithAttributes(commonLabels...))
			instruments.LineLengths.Record(ctx, randLineLength, metric.WithAttributes(commonLabels...))
			fmt.Printf("#%d: LineLength: %dBy\n", i, randLineLength)
		}

		instruments.RequestLatency.Record(ctx, latencyMs, metric.WithAttributes(commonLabels...))
		instruments.RequestCount.Add(ctx, 1, metric.WithAttributes(commonLabels...))
		fmt.Printf("Latency: %.3fms\n", latencyMs)
		time.Sleep(time.Duration(1) * time.Second)
	}
}

// makeRequest sends requests to the server using an OTEL HTTP transport which will instrument the requests with traces.
func makeRequest(ctx context.Context) {

	demoServerAddr, ok := os.LookupEnv("DEMO_SERVER_ENDPOINT")
	if !ok {
		demoServerAddr = "http://0.0.0.0:7080/hello"
	}

	// Trace an HTTP client by wrapping the transport
	client := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	// Make sure we pass the context to the request to avoid broken traces.
	req, err := http.NewRequestWithContext(ctx, "GET", demoServerAddr, nil)
	if err != nil {
		handleErr(err, "failed to http request")
	}

	// All requests made with this client will create spans.
	res, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	res.Body.Close()
}

// ClientInstruments is a collection of instruments used to measure client requests to the server
type ClientInstruments struct {
	RequestLatency metric.Float64Histogram
	RequestCount   metric.Int64Counter
	LineLengths    metric.Int64Histogram
	LineCounts     metric.Int64Counter
}

// NewClientInstruments takes a meter and builds a set of instruments to be used to measure client requests to the server.
func NewClientInstruments(meter metric.Meter) ClientInstruments {
	requestLatency, err := meter.Float64Histogram(
		"request_latency",
		metric.WithDescription("The latency of requests processed"),
	)
	handleErr(err, "failed to create request latency histogram")

	requestCount, err := meter.Int64Counter(
		"request_counts",
		metric.WithDescription("The number of requests processed"),
	)
	handleErr(err, "failed to create request latency histogram")

	lineLengths, err := meter.Int64Histogram(
		"line_lengths",
		metric.WithDescription("The lengths of the various lines in"),
	)
	handleErr(err, "failed to create line lengths histogram")

	lineCounts, err := meter.Int64Counter(
		"line_counts",
		metric.WithDescription("The counts of the lines in"),
	)
	handleErr(err, "failed to create line counts counter")

	return ClientInstruments{
		RequestLatency: requestLatency,
		RequestCount:   requestCount,
		LineLengths:    lineLengths,
		LineCounts:     lineCounts,
	}
}
