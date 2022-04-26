package opentelemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenTelemetrySetup(t *testing.T) {

	tests := []struct {
		desc    string
		config  Config
		wantErr bool
	}{
		{
			desc: "without configuration, HTTP By default",
		},
		{
			desc: "with HTTP configuration",
			config: Config{
				HTTP: &ConfigHTTP{},
			},
		},
		{
			desc: "with GRPC configuration",
			config: Config{
				GRPC: &ConfigGRPC{},
			},
		},
		{
			desc: "with both HTTP, and GRPC configuration",
			config: Config{
				HTTP: &ConfigHTTP{},
				GRPC: &ConfigGRPC{},
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			_, closer, err := test.config.Setup("testCompoment")
			if test.wantErr {
				require.Nil(t, closer)
				assert.Error(t, err)
			} else {
				require.NotNil(t, closer)
				assert.NoError(t, err)
			}
		})
	}
}
