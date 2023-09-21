package metrics

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Controller represents the controller to send metrics to.
type Controller interface {
	isController()
}

// OTELGRPC represents exporting to the "go.opentelemetry.io/otel/sdk/metric/controller/basic" controller.
type OTELGRPC struct {
	// Addr is the local address to export on.
	Addr string
}

func (o OTELGRPC) isController() {}

// Stop is used to stop OTEL metric handling.
type Stop func()

// Start is used to start OTEL metric handling.
func Start(ctx context.Context, c Controller) (Stop, error) {
	done, err := newController(ctx, c)
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}
	return func() {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		if err := done(ctx); err != nil {
			otel.Handle(err)
		}
	}, nil
}

func newController(ctx context.Context, c Controller) (func(context.Context) error, error) {
	switch v := c.(type) {
	case OTELGRPC:
		return otelGRPC(ctx, v)
	}
	return nil, fmt.Errorf("%T is not a valid Controller", c)
}

func otelGRPC(ctx context.Context, args OTELGRPC) (func(context.Context) error, error) {
	exp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(args.Addr),
	)
	if err != nil {
		panic(err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service", "petstore-server")),
		resource.WithHost(),
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String("petstore-server"),
		),
	)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)
	return func(doneCtx context.Context) error {
		// pushes any last exports to the receiver
		return meterProvider.Shutdown(ctx)
	}, nil
}
