package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/instana/testify/require"
	"github.com/stretchr/testify/assert"
	ptypes "github.com/traefik/paerser/types"
	"github.com/traefik/traefik/v2/pkg/types"
)

func TestOpenTelemetry_NewController(t *testing.T) {
	tests := []struct {
		desc    string
		config  types.OpenTelemetry
		wantErr bool
	}{
		{
			desc: "without configuration, HTTP By default",
			config: types.OpenTelemetry{
				CollectPeriod: ptypes.Duration(10 * time.Second),
			},
		},
		{
			desc: "with HTTP configuration",
			config: types.OpenTelemetry{
				HTTP:          &types.OTELHTTP{},
				CollectPeriod: ptypes.Duration(10 * time.Second),
			},
		},
		{
			desc: "with GRPC configuration",
			config: types.OpenTelemetry{
				GRPC:          &types.OTELGRPC{},
				CollectPeriod: ptypes.Duration(10 * time.Second),
			},
		},
		{
			desc: "with both HTTP, and GRPC configuration",
			config: types.OpenTelemetry{
				HTTP:          &types.OTELHTTP{},
				GRPC:          &types.OTELGRPC{},
				CollectPeriod: ptypes.Duration(10 * time.Second),
			},
			wantErr: true,
		},
		{
			desc: "with CollectPeriod set to 0",
			config: types.OpenTelemetry{
				CollectPeriod: 0,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			registery, err := newOpenTelemetryController(context.Background(), &test.config)
			if test.wantErr {
				assert.Error(t, err)
				require.Nil(t, registery)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, registery)
			}
		})
	}
}

func TestOpenTelemetry(t *testing.T) {
	c := make(chan *string)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		bodyStr := string(body)
		t.Log(bodyStr)
		c <- &bodyStr

		_, err = fmt.Fprintln(w, "ok")
		require.NoError(t, err)
	}))
	defer ts.Close()

	registry := RegisterOpenTelemetry(context.Background(), &types.OpenTelemetry{
		AddEntryPointsLabels: true,
		AddRoutersLabels:     true,
		AddServicesLabels:    true,
		CollectPeriod:        ptypes.Duration(time.Second),
		Endpoint:             ts.Listener.Addr().String(),
		Insecure:             true,
	})
	defer StopOpenTelemetry()

	require.NotNil(t, registry)

	if !registry.IsEpEnabled() || !registry.IsRouterEnabled() || !registry.IsSvcEnabled() {
		t.Fatalf("registry should return true for IsEnabled(), IsRouterEnabled() and IsSvcEnabled()")
	}

	expectedServer := []string{
		`(traefik_config_reloads_total\x12\x0eConfig reloads\x1a\x011)`,
		`(traefik_config_reloads_failure_total\x12\x16Config failure reloads\x1a\x011)`,
		`(traefik_config_last_reload_success\x12\x1aLast config reload success\x1a\x02ms)`,
		`(traefik_config_last_reload_failure\x12\x1aLast config reload failure\x1a\x02ms)`,
	}

	registry.ConfigReloadsCounter().Add(1)
	registry.ConfigReloadsFailureCounter().Add(1)
	registry.LastConfigReloadSuccessGauge().Set(1)
	registry.LastConfigReloadFailureGauge().Set(1)
	msgServer := <-c

	assertMessage(t, *msgServer, expectedServer)
}
