package failover

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/instana/testify/require"
	"github.com/stretchr/testify/assert"
	"github.com/traefik/traefik/v2/pkg/config/dynamic"
)

type responseRecorder struct {
	*httptest.ResponseRecorder
	save     map[string]int
	sequence []string
	status   []int
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.save[r.Header().Get("server")]++
	r.sequence = append(r.sequence, r.Header().Get("server"))
	r.status = append(r.status, statusCode)
	r.ResponseRecorder.WriteHeader(statusCode)
}

func TestBalancer(t *testing.T) {
	balancer := New(&dynamic.HealthCheck{})
	var balancerStatus bool
	require.NoError(t, balancer.RegisterStatusUpdater(func(up bool) {
		t.Logf("Updating status to %t\n", up)
		balancerStatus = up
	}))

	balancer.SetHandler(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("server", "handler")
		rw.WriteHeader(http.StatusOK)
	}))

	balancer.SetHandlerStatus(context.Background(), false)
	balancer.SetHandlerStatus(context.Background(), true)

	balancer.SetFailoverHandler(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("server", "failover")
		rw.WriteHeader(http.StatusOK)
	}))

	recorder := &responseRecorder{ResponseRecorder: httptest.NewRecorder(), save: map[string]int{}}
	for i := 0; i < 4; i++ {
		balancer.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	}

	assert.Equal(t, 4, recorder.save["handler"])
	assert.Equal(t, 0, recorder.save["failover"])
	assert.Equal(t, []int{200, 200, 200, 200}, recorder.status)
	assert.True(t, balancerStatus)

	balancer.SetHandlerStatus(context.Background(), false)

	recorder = &responseRecorder{ResponseRecorder: httptest.NewRecorder(), save: map[string]int{}}
	for i := 0; i < 4; i++ {
		balancer.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	}

	assert.Equal(t, 0, recorder.save["handler"])
	assert.Equal(t, 4, recorder.save["failover"])
	assert.Equal(t, []int{200, 200, 200, 200}, recorder.status)
	assert.True(t, balancerStatus)

	balancer.SetFailoverHandlerStatus(context.Background(), false)

	recorder = &responseRecorder{ResponseRecorder: httptest.NewRecorder(), save: map[string]int{}}
	for i := 0; i < 4; i++ {
		balancer.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	}

	assert.Equal(t, 0, recorder.save["handler"])
	assert.Equal(t, 0, recorder.save["failover"])
	assert.Equal(t, []int{503, 503, 503, 503}, recorder.status)
	assert.False(t, balancerStatus)
}
