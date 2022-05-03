package opentelemetry

import (
	"context"
	"errors"
	"io"

	"github.com/opentracing/opentracing-go"
	"github.com/traefik/traefik/v2/pkg/log"
	traefikversion "github.com/traefik/traefik/v2/pkg/version"
	"go.opentelemetry.io/otel"
	oteltracer "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/encoding/gzip"
)

// Setup sets up the tracer.
func (c *Config) Setup(componentName string) (opentracing.Tracer, io.Closer, error) {
	if c.HTTP != nil && c.GRPC != nil {
		return nil, nil, errors.New("cannot define HTTP and GRPC exporter concurrently")
	}

	if c.HTTP == nil && c.GRPC == nil {
		// return nil, nil, errors.New("cannot define Open Telemetry tracer without defining one of the HTTP or GRPC exporter configuration")
		c.HTTP = &ConfigHTTP{}
		c.HTTP.SetDefaults()
	}

	// Tracer
	bt := oteltracer.NewBridgeTracer()
	// TODO add schema URL
	bt.SetOpenTelemetryTracer(otel.Tracer(componentName,
		trace.WithInstrumentationVersion(traefikversion.Version)))
	opentracing.SetGlobalTracer(bt)

	var closer io.Closer
	var err error
	if c.HTTP != nil {
		closer, err = c.setupHTTPExporter(c.HTTP)
	} else {
		closer, err = c.setupGRPCExporter(c.GRPC)
	}

	return bt, closer, err
}

func (c *Config) setupGRPCExporter(grpc *ConfigGRPC) (io.Closer, error) {
	// TODO: handle TLSClientConfig, DialOption
	optsClient := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(c.Endpoint),
		otlptracegrpc.WithHeaders(c.Headers),
		otlptracegrpc.WithReconnectionPeriod(grpc.ReconnectionPeriod),
		otlptracegrpc.WithServiceConfig(grpc.ServiceConfig),
		otlptracegrpc.WithTimeout(c.Timeout),
	}

	if c.Compress {
		optsClient = append(optsClient, otlptracegrpc.WithCompressor(gzip.Name))
	}

	if c.Insecure {
		optsClient = append(optsClient, otlptracegrpc.WithInsecure())
	}

	if c.Retry != nil {
		optsClient = append(optsClient, otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: c.Retry.InitialInterval,
			MaxInterval:     c.Retry.MaxInterval,
			MaxElapsedTime:  c.Retry.MaxElapsedTime,
		}))
	}

	client := otlptracegrpc.NewClient(optsClient...)
	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	otel.SetTracerProvider(tracerProvider)

	log.WithoutContext().Debug("Opentracing GRPC tracer configured")

	return tpCloser{provider: tracerProvider}, nil
}

func (c *Config) setupHTTPExporter(http *ConfigHTTP) (io.Closer, error) {
	compress := otlptracehttp.NoCompression
	if c.Compress {
		compress = otlptracehttp.GzipCompression
	}

	// TODO: handle TLSClientConfig
	optsClient := []otlptracehttp.Option{
		otlptracehttp.WithCompression(compress),
		otlptracehttp.WithEndpoint(c.Endpoint),
		otlptracehttp.WithHeaders(c.Headers),
		otlptracehttp.WithTimeout(c.Timeout),
		otlptracehttp.WithURLPath(http.URLPath),
	}

	if c.Insecure {
		optsClient = append(optsClient, otlptracehttp.WithInsecure())
	}

	if c.Retry != nil {
		optsClient = append(optsClient, otlptracehttp.WithRetry(otlptracehttp.RetryConfig{
			Enabled:         true,
			InitialInterval: c.Retry.InitialInterval,
			MaxInterval:     c.Retry.MaxInterval,
			MaxElapsedTime:  c.Retry.MaxElapsedTime,
		}))
	}

	client := otlptracehttp.NewClient(optsClient...)
	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	otel.SetTracerProvider(tracerProvider)

	log.WithoutContext().Debug("Opentracing HTTP tracer configured")

	return tpCloser{provider: tracerProvider}, nil
}

type tpCloser struct {
	provider *sdktrace.TracerProvider
}

func (t tpCloser) Close() error {
	return t.provider.Shutdown(context.Background())
}
