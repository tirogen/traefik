package metrics

import (
	"context"
	"encoding/json"
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
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
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
				PushInterval: ptypes.Duration(10 * time.Second),
			},
		},
		{
			desc: "with HTTP configuration",
			config: types.OpenTelemetry{
				HTTP:         &types.OTELHTTP{},
				PushInterval: ptypes.Duration(10 * time.Second),
			},
		},
		{
			desc: "with GRPC configuration",
			config: types.OpenTelemetry{
				GRPC:         &types.OTELGRPC{},
				PushInterval: ptypes.Duration(10 * time.Second),
			},
		},
		{
			desc: "with both HTTP, and GRPC configuration",
			config: types.OpenTelemetry{
				HTTP:         &types.OTELHTTP{},
				GRPC:         &types.OTELGRPC{},
				PushInterval: ptypes.Duration(10 * time.Second),
			},
			wantErr: true,
		},
		{
			desc: "with PushInterval set to 0",
			config: types.OpenTelemetry{
				PushInterval: 0,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			registry, err := newOpenTelemetryController(context.Background(), &test.config)
			if test.wantErr {
				assert.Error(t, err)
				require.Nil(t, registry)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, registry)
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

		req := pmetricotlp.NewRequest()
		err = req.UnmarshalProto(body)
		require.NoError(t, err)

		marshalledReq, err := json.Marshal(req)
		require.NoError(t, err)

		bodyStr := string(marshalledReq)
		c <- &bodyStr

		_, err = fmt.Fprintln(w, "ok")
		require.NoError(t, err)
	}))
	defer ts.Close()

	var cfg types.OpenTelemetry
	(&cfg).SetDefaults()
	cfg.AddRoutersLabels = true
	cfg.Insecure = true
	cfg.Address = ts.Listener.Addr().String()
	cfg.PushInterval = ptypes.Duration(time.Millisecond)

	registry := RegisterOpenTelemetry(context.Background(), &cfg)
	defer StopOpenTelemetry()

	require.NotNil(t, registry)

	if !registry.IsEpEnabled() || !registry.IsRouterEnabled() || !registry.IsSvcEnabled() {
		t.Fatalf("registry should return true for IsEnabled(), IsRouterEnabled() and IsSvcEnabled()")
	}

	expectedServer := []string{
		`({"name":"traefik_config_reloads_total","description":"Config reloads","unit":"1","sum":{"dataPoints":\[{"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_config_reloads_failure_total","description":"Config failure reloads","unit":"1","sum":{"dataPoints":\[{"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_config_last_reload_success","description":"Last config reload success","unit":"ms","gauge":{"dataPoints":\[{"timeUnixNano":"[\d]{19}","asDouble":1}\]}})`,
		`({"name":"traefik_config_last_reload_failure","description":"Last config reload failure","unit":"ms","gauge":{"dataPoints":\[{"timeUnixNano":"[\d]{19}","asDouble":1}\]}})`,
	}

	registry.ConfigReloadsCounter().Add(1)
	registry.ConfigReloadsFailureCounter().Add(1)
	registry.LastConfigReloadSuccessGauge().Set(1)
	registry.LastConfigReloadFailureGauge().Set(1)
	msgServer := <-c

	assertMessage(t, *msgServer, expectedServer)

	expectedTLS := []string{
		`({"name":"traefik_tls_certs_not_after","description":"Certificate expiration timestamp","unit":"ms","gauge":{"dataPoints":\[{"attributes":\[{"key":"key","value":{"stringValue":"value"}}\],"timeUnixNano":"[\d]{19}","asDouble":1}\]}})`,
	}

	registry.TLSCertsNotAfterTimestampGauge().With("key", "value").Set(1)
	msgTLS := <-c

	assertMessage(t, *msgTLS, expectedTLS)

	expectedEntrypoint := []string{
		`({"name":"traefik_entrypoint_requests_total","description":"How many HTTP requests processed on an entrypoint, partitioned by status code, protocol, and method.","unit":"1","sum":{"dataPoints":\[{"attributes":\[{"key":"code","value":{"stringValue":"200"}},{"key":"entrypoint","value":{"stringValue":"test1"}},{"key":"method","value":{"stringValue":"GET"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_entrypoint_requests_tls_total","description":"How many HTTP requests with TLS processed on an entrypoint, partitioned by TLS Version and TLS cipher Used.","unit":"1","sum":{"dataPoints":\[{"attributes":\[{"key":"entrypoint","value":{"stringValue":"test2"}},{"key":"tls_cipher","value":{"stringValue":"bar"}},{"key":"tls_version","value":{"stringValue":"foo"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_entrypoint_request_duration_seconds","description":"How long it took to process the request on an entrypoint, partitioned by status code, protocol, and method.","unit":"ms","histogram":{"dataPoints":\[{"attributes":\[{"key":"entrypoint","value":{"stringValue":"test3"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","count":"1","sum":10000,"bucketCounts":\["0","0","0","0","0","0","0","0","0","0","0","1"\],"explicitBounds":\[0.005,0.01,0.025,0.05,0.1,0.25,0.5,1,2.5,5,10\]}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_entrypoint_open_connections","description":"How many open connections exist on an entrypoint, partitioned by method and protocol.","unit":"1","gauge":{"dataPoints":\[{"attributes":\[{"key":"entrypoint","value":{"stringValue":"test4"}}\],"timeUnixNano":"[\d]{19}","asDouble":1}\]}})`,
	}

	registry.EntryPointReqsCounter().With("entrypoint", "test1", "code", strconv.Itoa(http.StatusOK), "method", http.MethodGet).Add(1)
	registry.EntryPointReqsTLSCounter().With("entrypoint", "test2", "tls_version", "foo", "tls_cipher", "bar").Add(1)
	registry.EntryPointReqDurationHistogram().With("entrypoint", "test3").Observe(10000)
	registry.EntryPointOpenConnsGauge().With("entrypoint", "test4").Set(1)
	msgEntrypoint := <-c

	assertMessage(t, *msgEntrypoint, expectedEntrypoint)

	expectedRouter := []string{
		`({"name":"traefik_router_requests_total","description":"How many HTTP requests are processed on a router, partitioned by service, status code, protocol, and method.","unit":"1","sum":{"dataPoints":\[{"attributes":\[{"key":"code","value":{"stringValue":"(?:200|404)"}},{"key":"method","value":{"stringValue":"GET"}},{"key":"router","value":{"stringValue":"RouterReqsCounter"}},{"key":"service","value":{"stringValue":"test"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1},{"attributes":\[{"key":"code","value":{"stringValue":"(?:200|404)"}},{"key":"method","value":{"stringValue":"GET"}},{"key":"router","value":{"stringValue":"RouterReqsCounter"}},{"key":"service","value":{"stringValue":"test"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_router_requests_tls_total","description":"How many HTTP requests with TLS are processed on a router, partitioned by service, TLS Version, and TLS cipher Used.","unit":"1","sum":{"dataPoints":\[{"attributes":\[{"key":"router","value":{"stringValue":"demo"}},{"key":"service","value":{"stringValue":"test"}},{"key":"tls_cipher","value":{"stringValue":"bar"}},{"key":"tls_version","value":{"stringValue":"foo"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_router_request_duration_seconds","description":"How long it took to process the request on a router, partitioned by service, status code, protocol, and method.","unit":"ms","histogram":{"dataPoints":\[{"attributes":\[{"key":"code","value":{"stringValue":"200"}},{"key":"router","value":{"stringValue":"demo"}},{"key":"service","value":{"stringValue":"test"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","count":"1","sum":10000,"bucketCounts":\["0","0","0","0","0","0","0","0","0","0","0","1"\],"explicitBounds":\[0.005,0.01,0.025,0.05,0.1,0.25,0.5,1,2.5,5,10\]}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_router_open_connections","description":"How many open connections exist on a router, partitioned by service, method, and protocol.","unit":"1","gauge":{"dataPoints":\[{"attributes":\[{"key":"router","value":{"stringValue":"demo"}},{"key":"service","value":{"stringValue":"test"}}\],"timeUnixNano":"[\d]{19}","asDouble":1}\]}})`,
	}

	registry.RouterReqsCounter().With("router", "RouterReqsCounter", "service", "test", "code", strconv.Itoa(http.StatusNotFound), "method", http.MethodGet).Add(1)
	registry.RouterReqsCounter().With("router", "RouterReqsCounter", "service", "test", "code", strconv.Itoa(http.StatusOK), "method", http.MethodGet).Add(1)
	registry.RouterReqsTLSCounter().With("router", "demo", "service", "test", "tls_version", "foo", "tls_cipher", "bar").Add(1)
	registry.RouterReqDurationHistogram().With("router", "demo", "service", "test", "code", strconv.Itoa(http.StatusOK)).Observe(10000)
	registry.RouterOpenConnsGauge().With("router", "demo", "service", "test").Set(1)
	msgRouter := <-c

	assertMessage(t, *msgRouter, expectedRouter)

	expectedService := []string{
		`({"name":"traefik_service_requests_total","description":"How many HTTP requests processed on a service, partitioned by status code, protocol, and method.","unit":"1","sum":{"dataPoints":\[{"attributes":\[{"key":"code","value":{"stringValue":"(?:200|404)"}},{"key":"method","value":{"stringValue":"GET"}},{"key":"service","value":{"stringValue":"ServiceReqsCounter"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1},{"attributes":\[{"key":"code","value":{"stringValue":"(?:200|404)"}},{"key":"method","value":{"stringValue":"GET"}},{"key":"service","value":{"stringValue":"ServiceReqsCounter"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_service_requests_tls_total","description":"How many HTTP requests with TLS processed on a service, partitioned by TLS version and TLS cipher.","unit":"1","sum":{"dataPoints":\[{"attributes":\[{"key":"service","value":{"stringValue":"test"}},{"key":"tls_cipher","value":{"stringValue":"bar"}},{"key":"tls_version","value":{"stringValue":"foo"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_service_request_duration_seconds","description":"How long it took to process the request on a service, partitioned by status code, protocol, and method.","unit":"ms","histogram":{"dataPoints":\[{"attributes":\[{"key":"code","value":{"stringValue":"200"}},{"key":"service","value":{"stringValue":"test"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","count":"1","sum":10000,"bucketCounts":\["0","0","0","0","0","0","0","0","0","0","0","1"\],"explicitBounds":\[0.005,0.01,0.025,0.05,0.1,0.25,0.5,1,2.5,5,10\]}\],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE"}})`,
		`({"name":"traefik_service_server_up","description":"service server is up, described by gauge value of 0 or 1.","unit":"1","gauge":{"dataPoints":\[{"attributes":\[{"key":"service","value":{"stringValue":"test"}},{"key":"url","value":{"stringValue":"http://127.0.0.1"}}\],"timeUnixNano":"[\d]{19}","asDouble":1}\]}})`,
	}

	registry.ServiceReqsCounter().With("service", "ServiceReqsCounter", "code", strconv.Itoa(http.StatusOK), "method", http.MethodGet).Add(1)
	registry.ServiceReqsCounter().With("service", "ServiceReqsCounter", "code", strconv.Itoa(http.StatusNotFound), "method", http.MethodGet).Add(1)
	registry.ServiceReqsTLSCounter().With("service", "test", "tls_version", "foo", "tls_cipher", "bar").Add(1)
	registry.ServiceReqDurationHistogram().With("service", "test", "code", strconv.Itoa(http.StatusOK)).Observe(10000)
	registry.ServiceServerUpGauge().With("service", "test", "url", "http://127.0.0.1").Set(1)
	msgService := <-c

	assertMessage(t, *msgService, expectedService)

	expectedServiceRetries := []string{
		`({"attributes":\[{"key":"service","value":{"stringValue":"foobar"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":1})`,
		`({"attributes":\[{"key":"service","value":{"stringValue":"test"}}\],"startTimeUnixNano":"[\d]{19}","timeUnixNano":"[\d]{19}","asDouble":2})`,
	}

	registry.ServiceRetriesCounter().With("service", "test").Add(1)
	registry.ServiceRetriesCounter().With("service", "test").Add(1)
	registry.ServiceRetriesCounter().With("service", "foobar").Add(1)

	msgServiceRetries := <-c

	assertMessage(t, *msgServiceRetries, expectedServiceRetries)

	expectedServiceOpenConns := []string{
		`({"attributes":\[{"key":"service","value":{"stringValue":"foobar"}}\],"timeUnixNano":"[\d]{19}","asDouble":1})`,
		`({"attributes":\[{"key":"service","value":{"stringValue":"test"}}\],"timeUnixNano":"[\d]{19}","asDouble":1})`,
	}

	registry.ServiceOpenConnsGauge().With("service", "test").Add(1)
	registry.ServiceOpenConnsGauge().With("service", "test").Add(1)
	registry.ServiceOpenConnsGauge().With("service", "foobar").Add(1)

	msgServiceOpenConns := <-c

	assertMessage(t, *msgServiceOpenConns, expectedServiceOpenConns)
}
