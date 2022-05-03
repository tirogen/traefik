package opentelemetry

import (
	"time"
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
