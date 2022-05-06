package opentelemetry

import (
	"context"
	"io"
	"net/url"

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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"
)

// Setup sets up the tracer.
func (c *Config) Setup(componentName string) (opentracing.Tracer, io.Closer, error) {
	// Tracer
	bt := oteltracer.NewBridgeTracer()
	// TODO add schema URL
	bt.SetOpenTelemetryTracer(otel.Tracer(componentName,
		trace.WithInstrumentationVersion(traefikversion.Version)))
	opentracing.SetGlobalTracer(bt)

	var closer io.Closer
	var err error
	if c.GRPC != nil {
		closer, err = c.setupGRPCExporter(c.GRPC)
	} else {
		closer, err = c.setupHTTPExporter()
	}

	return bt, closer, err
}

func (c *Config) setupHTTPExporter() (io.Closer, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, err
	}

	compress := otlptracehttp.NoCompression
	if c.Compress {
		compress = otlptracehttp.GzipCompression
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithCompression(compress),
		otlptracehttp.WithEndpoint(u.Host),
		otlptracehttp.WithHeaders(c.Headers),
		otlptracehttp.WithTimeout(c.Timeout),
	}

	if u.Scheme == "http" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	// https://github.com/open-telemetry/opentelemetry-go/blob/exporters/otlp/otlpmetric/v0.30.0/exporters/otlp/otlptrace/internal/otlpconfig/options.go#L35
	path := "/v1/traces"
	if u.Path != "" {
		path = u.Path
	}
	opts = append(opts, otlptracehttp.WithURLPath(path))

	if c.Retry != nil {
		opts = append(opts, otlptracehttp.WithRetry(otlptracehttp.RetryConfig{
			Enabled:         true,
			InitialInterval: c.Retry.InitialInterval,
			MaxInterval:     c.Retry.MaxInterval,
			MaxElapsedTime:  c.Retry.MaxElapsedTime,
		}))
	}

	if c.TLS != nil {
		tlsConfig, err := c.TLS.CreateTLSConfig(context.Background())
		if err != nil {
			return nil, err
		}

		opts = append(opts, otlptracehttp.WithTLSClientConfig(tlsConfig))
	}

	client := otlptracehttp.NewClient(opts...)
	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	otel.SetTracerProvider(tracerProvider)

	log.WithoutContext().Debug("Opentracing HTTP tracer configured")

	return tpCloser{provider: tracerProvider}, nil
}

func (c *Config) setupGRPCExporter(grpc *ConfigGRPC) (io.Closer, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, err
	}

	// TODO: handle DialOption
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(u.Host),
		otlptracegrpc.WithHeaders(c.Headers),
		otlptracegrpc.WithReconnectionPeriod(grpc.ReconnectionPeriod),
		otlptracegrpc.WithServiceConfig(grpc.ServiceConfig),
		otlptracegrpc.WithTimeout(c.Timeout),
	}

	if c.Compress {
		opts = append(opts, otlptracegrpc.WithCompressor(gzip.Name))
	}

	if grpc.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	if c.Retry != nil {
		opts = append(opts, otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: c.Retry.InitialInterval,
			MaxInterval:     c.Retry.MaxInterval,
			MaxElapsedTime:  c.Retry.MaxElapsedTime,
		}))
	}

	if c.TLS != nil {
		tlsConfig, err := c.TLS.CreateTLSConfig(context.Background())
		if err != nil {
			return nil, err
		}

		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)))
	}

	client := otlptracegrpc.NewClient(opts...)
	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	otel.SetTracerProvider(tracerProvider)

	log.WithoutContext().Debug("Opentracing GRPC tracer configured")

	return tpCloser{provider: tracerProvider}, nil
}

// tpCloser converts a TraceProvider into an io.Closer
type tpCloser struct {
	provider *sdktrace.TracerProvider
}

func (t tpCloser) Close() error {
	return t.provider.Shutdown(context.Background())
}