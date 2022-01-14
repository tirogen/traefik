package instana

import (
	"io"

	instana "github.com/instana/go-sensor"
	"github.com/opentracing/opentracing-go"
	"github.com/traefik/traefik/v2/pkg/log"
)

// Name sets the name of this tracer.
const Name = "instana"

// Config provides configuration settings for a instana tracer.
type Config struct {
	LocalAgentHost string `description:"Sets the Instana Agent host." json:"localAgentHost,omitempty" toml:"localAgentHost,omitempty" yaml:"localAgentHost,omitempty" loggable:"true"`
	LocalAgentPort int    `description:"Sets the Instana Agent port used." json:"localAgentPort,omitempty" toml:"localAgentPort,omitempty" yaml:"localAgentPort,omitempty" loggable:"true"`
	LogLevel       string `description:"Sets the log level for the Instana tracer. ('error','warn','info','debug')" json:"logLevel,omitempty" toml:"logLevel,omitempty" yaml:"logLevel,omitempty" export:"true" loggable:"true"`
}

// SetDefaults sets the default values.
func (c *Config) SetDefaults() {
	c.LocalAgentPort = 42699
	c.LogLevel = "info"
}

// Setup sets up the tracer.
func (c *Config) Setup(serviceName string) (opentracing.Tracer, io.Closer, error) {
	// set default logLevel
	logLevel := instana.Info

	// check/set logLevel overrides
	switch c.LogLevel {
	case "error":
		logLevel = instana.Error
	case "warn":
		logLevel = instana.Warn
	case "debug":
		logLevel = instana.Debug
	}

	tracer := instana.NewTracerWithOptions(&instana.Options{
		Service:   serviceName,
		LogLevel:  logLevel,
		AgentPort: c.LocalAgentPort,
		AgentHost: c.LocalAgentHost,
	})

	// Without this, child spans are getting the NOOP tracer
	opentracing.SetGlobalTracer(tracer)

	log.WithoutContext().Debug("Instana tracer configured")

	return tracer, nil, nil
}
