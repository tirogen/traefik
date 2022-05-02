package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/instana/testify/require"
	"github.com/stretchr/testify/assert"
	ptypes "github.com/traefik/paerser/types"
	"github.com/traefik/traefik/v2/pkg/types"
	"go.opentelemetry.io/otel/attribute"
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

func TestOpenTelemetry_labels(t *testing.T) {
	tests := []struct {
		desc   string
		values otelLabelNamesValues
		with   []string
		expect []attribute.KeyValue
	}{
		{
			desc:   "with no starting value",
			values: otelLabelNamesValues{},
			expect: []attribute.KeyValue{},
		},
		{
			desc:   "with one starting value",
			values: otelLabelNamesValues{"foo"},
			expect: []attribute.KeyValue{},
		},
		{
			desc:   "with two starting value",
			values: otelLabelNamesValues{"foo", "bar"},
			expect: []attribute.KeyValue{attribute.String("foo", "bar")},
		},
		{
			desc:   "with no starting value, and with one other value",
			values: otelLabelNamesValues{},
			with:   []string{"baz"},
			expect: []attribute.KeyValue{attribute.String("baz", "unknown")},
		},
		{
			desc:   "with no starting value, and with two other value",
			values: otelLabelNamesValues{},
			with:   []string{"baz", "buz"},
			expect: []attribute.KeyValue{attribute.String("baz", "buz")},
		},
		{
			desc:   "with one starting value, and with one other value",
			values: otelLabelNamesValues{"foo"},
			with:   []string{"baz"},
			expect: []attribute.KeyValue{attribute.String("foo", "baz")},
		},
		{
			desc:   "with one starting value, and with two other value",
			values: otelLabelNamesValues{"foo"},
			with:   []string{"baz", "buz"},
			expect: []attribute.KeyValue{attribute.String("foo", "baz")},
		},
		{
			desc:   "with two starting value, and with one other value",
			values: otelLabelNamesValues{"foo", "bar"},
			with:   []string{"baz"},
			expect: []attribute.KeyValue{
				attribute.String("foo", "bar"),
				attribute.String("baz", "unknown"),
			},
		},
		{
			desc:   "with two starting value, and with two other value",
			values: otelLabelNamesValues{"foo", "bar"},
			with:   []string{"baz", "buz"},
			expect: []attribute.KeyValue{
				attribute.String("foo", "bar"),
				attribute.String("baz", "buz"),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			assert.Equal(t, test.expect, test.values.With(test.with...).ToLabels())
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

	var cfg types.OpenTelemetry
	(&cfg).SetDefaults()
	cfg.AddRoutersLabels = true
	cfg.Insecure = true
	cfg.Endpoint = ts.Listener.Addr().String()

	registry := RegisterOpenTelemetry(context.Background(), &cfg)
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

	expectedTLS := []string{
		`(traefik_tls_certs_not_after\x12 Certificate expiration timestamp\x1a\x02ms)`, `(key\x12\a\n\x05value)`,
	}

	registry.TLSCertsNotAfterTimestampGauge().With("key", "value").Set(1)
	msgTLS := <-c

	assertMessage(t, *msgTLS, expectedTLS)

	expectedEntrypoint := []string{
		`(traefik_entrypoint_requests_total\x12dHow many HTTP requests processed on an entrypoint, partitioned by status code, protocol, and method.\x1a\x011)`, `(code\x12\x05\n\x03200:\x15\n\nentrypoint\x12\a\n\x05test1:\x0f\n\x06method\x12\x05\n\x03GET)`,
		`(traefik_entrypoint_requests_tls_total\x12kHow many HTTP requests with TLS processed on an entrypoint, partitioned by TLS Version and TLS cipher Used.\x1a\x011)`, `(tls_cipher\x12\x05\n\x03bar:\x14\n\vtls_version\x12\x05\n\x03foo)`,
		`(traefik_entrypoint_request_duration_seconds\x12kHow long it took to process the request on an entrypoint, partitioned by status code, protocol, and method.)`, `(entrypoint\x12\a\n\x05test3)`,
		`(traefik_entrypoint_open_connections\x12UHow many open connections exist on an entrypoint, partitioned by method and protocol.\x1a\x011*F\nD\x19\xfef\xcbO\x94f\xea\x16)`, `()`,
	}

	registry.EntryPointReqsCounter().With("entrypoint", "test1", "code", strconv.Itoa(http.StatusOK), "method", http.MethodGet).Add(1)
	registry.EntryPointReqsTLSCounter().With("entrypoint", "test2", "tls_version", "foo", "tls_cipher", "bar").Add(1)
	registry.EntryPointReqDurationHistogram().With("entrypoint", "test3").Observe(10000)
	registry.EntryPointOpenConnsGauge().With("entrypoint", "test4").Set(1)
	msgEntrypoint := <-c

	assertMessage(t, *msgEntrypoint, expectedEntrypoint)
}
