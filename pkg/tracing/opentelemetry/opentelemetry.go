package opentelemetry

import (
	"context"
	"io"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/traefik/traefik/v2/pkg/log"
	traefikversion "github.com/traefik/traefik/v2/pkg/version"
	"go.opentelemetry.io/otel"
	oteltracer "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Config provides configuration settings for an opentelemetry tracer.
type Config struct {
	Compress bool              `description:"Enable compression on the sent data." json:"compress,omitempty" toml:"compress,omitempty" yaml:"compress,omitempty" export:"true"`
	Endpoint string            `description:"Address of the collector endpoint." json:"endpoint,omitempty" toml:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Headers  map[string]string `description:"Headers sent with payload." json:"headers,omitempty" toml:"headers,omitempty" yaml:"headers,omitempty" export:"true"`
	Insecure bool              `description:"Connect to endpoint using HTTP." json:"insecure,omitempty" toml:"insecure,omitempty" yaml:"insecure,omitempty" export:"true"`
	Retry    *retry            `description:"The retry policy for transient errors that may occurs when exporting traces." json:"rety,omitempty" toml:"rety,omitempty" yaml:"rety,omitempty" export:"true"`
	Timeout  time.Duration     `description:"The max waiting time for the backend to process each spans batch." json:"timeout,omitempty" toml:"timeout,omitempty" yaml:"timeout,omitempty" export:"true"`
	URLPath  string            `description:"Override the default URL path used for sending traces." json:"urlPath,omitempty" toml:"urlPath,omitempty" yaml:"urlPath,omitempty"`
}

// SetDefaults sets the default values.
func (c *Config) SetDefaults() {
	c.Endpoint = "localhost:4318"
	c.Retry = &retry{}
	c.Retry.SetDefaults()
	c.Timeout = 10 * time.Second
	c.URLPath = "/v1/traces"
}

type retry struct {
	InitialInterval time.Duration `description:"The time to wait after the first failure before retrying." json:"initialInterval,omitempty" toml:"initialInterval,omitempty" yaml:"initialInterval,omitempty" export:"true"`
	MaxInterval     time.Duration `description:"The upper bound on backoff interval." json:"maxInterval,omitempty" toml:"maxInterval,omitempty" yaml:"maxInterval,omitempty" export:"true"`
	MaxElapsedTime  time.Duration `description:"The maximum amount of time (including retries) spent trying to send a request/batch." json:"maxElapsedTime,omitempty" toml:"maxElapsedTime,omitempty" yaml:"maxElapsedTime,omitempty" export:"true"`
}

// SetDefaults sets the default values.
func (r *retry) SetDefaults() {
	r.InitialInterval = 5 * time.Second
	r.MaxInterval = 30 * time.Second
	r.MaxElapsedTime = time.Minute
}

// Setup sets up the tracer.
func (c *Config) Setup(componentName string) (opentracing.Tracer, io.Closer, error) {
	// Tracer
	// TODO add schema URL
	opts := []trace.TracerOption{
		trace.WithInstrumentationVersion(traefikversion.Version),
	}
	bt := oteltracer.NewBridgeTracer()
	bt.SetOpenTelemetryTracer(otel.Tracer(componentName, opts...))
	opentracing.SetGlobalTracer(bt)
	log.WithoutContext().Debug("Opentracing tracer configured")

	// exporter
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
		otlptracehttp.WithURLPath(c.URLPath),
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
		return nil, nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	otel.SetTracerProvider(tracerProvider)
	log.WithoutContext().Debug("Opentracing exporter configured")

	return bt, tpCloser{provider: tracerProvider}, nil
}

type tpCloser struct {
	provider *sdktrace.TracerProvider
}

func (t tpCloser) Close() error {
	return t.provider.Shutdown(context.Background())
}
