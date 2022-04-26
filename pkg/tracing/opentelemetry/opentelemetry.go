package opentelemetry

import (
	"context"
	"errors"
	"io"
	"time"

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

// Config provides configuration settings for an open-telemetry tracer.
type Config struct {
	HTTP *ConfigHTTP `description:"HTTP specific configuration for the OpenTelemetry collector." json:"http,omitempty" toml:"http,omitempty" yaml:"http,omitempty" export:"true" label:"allowEmpty" file:"allowEmpty"`
	GRPC *ConfigGRPC `description:"GRPC specific configuration for the OpenTelemetry collector." json:"grpc,omitempty" toml:"grpc,omitempty" yaml:"grpc,omitempty" export:"true" label:"allowEmpty" file:"allowEmpty"`

	Compress bool              `description:"Enable compression on the sent data." json:"compress,omitempty" toml:"compress,omitempty" yaml:"compress,omitempty" export:"true"`
	Endpoint string            `description:"Address of the collector endpoint." json:"endpoint,omitempty" toml:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Headers  map[string]string `description:"Headers sent with payload." json:"headers,omitempty" toml:"headers,omitempty" yaml:"headers,omitempty" export:"true"`
	Insecure bool              `description:"Connect to endpoint using HTTP." json:"insecure,omitempty" toml:"insecure,omitempty" yaml:"insecure,omitempty" export:"true"`
	Retry    *retry            `description:"The retry policy for transient errors that may occurs when exporting traces." json:"retry,omitempty" toml:"retry,omitempty" yaml:"retry,omitempty" export:"true"`
	Timeout  time.Duration     `description:"The max waiting time for the backend to process each spans batch." json:"timeout,omitempty" toml:"timeout,omitempty" yaml:"timeout,omitempty" export:"true"`
}

// SetDefaults sets the default values.
func (c *Config) SetDefaults() {
	c.Endpoint = "localhost:4318"
	c.Retry = &retry{}
	c.Retry.SetDefaults()
	c.Timeout = 10 * time.Second
}

// ConfigHTTP provides configuration settings for an open-telemetry tracer.
type ConfigHTTP struct {
	URLPath string `description:"Override the default URL path used for sending traces." json:"urlPath,omitempty" toml:"urlPath,omitempty" yaml:"urlPath,omitempty"`
}

// SetDefaults sets the default values.
func (c *ConfigHTTP) SetDefaults() {
	c.URLPath = "/v1/traces"
}

// ConfigGRPC provides configuration settings for an open-telemetry tracer.
type ConfigGRPC struct {
	ReconnectionPeriod time.Duration `description:"The minimum amount of time between connection attempts to the target endpoint." json:"reconnectionPeriod,omitempty" toml:"reconnectionPeriod,omitempty" yaml:"reconnectionPeriod,omitempty" export:"true"`
	ServiceConfig      string        `description:"Defines the default gRPC service config used." json:"serviceConfig,omitempty" toml:"serviceConfig,omitempty" yaml:"serviceConfig,omitempty" export:"true"`
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
