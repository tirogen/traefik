package opentelemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
	middlewareTracing "github.com/traefik/traefik/v2/pkg/middlewares/tracing"
	"github.com/traefik/traefik/v2/pkg/tracing"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

func TestSpansCtx(t *testing.T) {
	uuid := "[0-9a-z]{32}" // FIXME

	tests := []struct {
		desc            string
		incomingHeaders http.Header
		outgoingHeaders http.Header
		match           string
	}{
		{
			desc: "no headers",
		},
		{
			desc: "With tracestate",
			incomingHeaders: http.Header{
				http.CanonicalHeaderKey("traceparent"): []string{uuid},
			},
			match: `{"traceId":"` + uuid + `","spanId":"[0-9a-z]{16}","parentSpanId":"","name":"EntryPoint testEP 127.0.0.1:\d+","kind":"SPAN_KIND_INTERNAL","startTimeUnixNano":"[\d]{19}","endTimeUnixNano":"[\d]{19}","attributes":\[{"key":"component","value":{"stringValue":"testService"}},{"key":"http.method","value":{"stringValue":"GET"}},{"key":"http.url","value":{"stringValue":"http://127.0.0.1:\d+"}},{"key":"http.host","value":{"stringValue":"127.0.0.1:\d+"}},{"key":"http.status_code","value":{"stringValue":"200"}}\],"status":{}}`,
		},
	}
	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			c := make(chan *string)
			defer close(c)
			spanServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				defer r.Body.Close()

				req := ptraceotlp.NewRequest()
				err = req.UnmarshalProto(body)
				require.NoError(t, err)

				marshalledReq, err := json.Marshal(req)
				require.NoError(t, err)

				bodyStr := string(marshalledReq)
				c <- &bodyStr

				_, err = fmt.Fprintln(w, "ok")
				require.NoError(t, err)
			}))
			defer spanServer.Close()

			config := Config{
				Endpoint: spanServer.URL,
			}

			tracer, err := tracing.NewTracing("testService", 0, &config)
			defer tracer.Close()
			require.NoError(t, err)

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := fmt.Fprintln(w, "ok")
				require.NoError(t, err)
			})
			wrapper := middlewareTracing.NewEntryPoint(context.Background(), tracer, "testEP", handler)
			testServer := httptest.NewServer(wrapper)
			defer testServer.Close()

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, testServer.URL, nil)
			req.Header = test.incomingHeaders
			wrapper.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusOK, recorder.Code)

			msgSpan := <-c
			assert.Matches(t, *msgSpan, test.match)
		})
	}
}
