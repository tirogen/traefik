package failover

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/traefik/traefik/v2/pkg/config/dynamic"
	"github.com/traefik/traefik/v2/pkg/log"
)

type Failover struct {
	wantsHealthCheck bool
	handler          http.Handler
	failoverHandler  http.Handler
	// updaters is the list of hooks that are run (to update the Balancer
	// parent(s)), whenever the Balancer status changes.
	updaters []func(bool)

	handlerStatusMu sync.RWMutex
	handlerStatus   bool

	failoverStatusMu sync.RWMutex
	failoverStatus   bool
}

func New(hc *dynamic.HealthCheck) *Failover {
	return &Failover{
		wantsHealthCheck: hc != nil,
	}
}

// RegisterStatusUpdater adds fn to the list of hooks that are run when the
// status of the Balancer changes.
// Not thread safe.
func (f *Failover) RegisterStatusUpdater(fn func(up bool)) error {
	if !f.wantsHealthCheck {
		return errors.New("healthCheck not enabled in config for this failover service")
	}

	f.updaters = append(f.updaters, fn)

	return nil
}

func (f *Failover) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	f.handlerStatusMu.RLock()
	handlerStatus := f.handlerStatus
	f.handlerStatusMu.RUnlock()

	if handlerStatus {
		f.handler.ServeHTTP(w, req)
		return
	}

	f.failoverStatusMu.RLock()
	failoverStatus := f.failoverStatus
	f.failoverStatusMu.RUnlock()

	if failoverStatus {
		f.failoverHandler.ServeHTTP(w, req)
		return
	}

	http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
}

func (f *Failover) SetHandler(handler http.Handler) {
	f.handlerStatusMu.Lock()
	defer f.handlerStatusMu.Unlock()

	f.handler = handler
	f.handlerStatus = true
}

func (f *Failover) SetFailoverHandler(handler http.Handler) {
	f.failoverStatusMu.Lock()
	defer f.failoverStatusMu.Unlock()

	f.failoverHandler = handler
	f.failoverStatus = true
}

func (f *Failover) SetHandlerStatus(ctx context.Context, up bool) {
	f.handlerStatusMu.Lock()
	defer f.handlerStatusMu.Unlock()

	if up == f.handlerStatus {
		// We're still with the same status, no need to propagate
		log.FromContext(ctx).Debugf("Still %t, no need to propagate", up)
		return
	}

	log.FromContext(ctx).Debugf("Propagating new %t status", up)
	f.handlerStatus = up

	for _, fn := range f.updaters {
		fn(f.handlerStatus || f.failoverStatus)
	}
}

func (f *Failover) SetFailoverHandlerStatus(ctx context.Context, up bool) {
	f.failoverStatusMu.Lock()
	defer f.failoverStatusMu.Unlock()

	if up == f.failoverStatus {
		// We're still with the same status, no need to propagate
		log.FromContext(ctx).Debugf("Still %t, no need to propagate", up)
		return
	}

	log.FromContext(ctx).Debugf("Propagating new %t status", up)
	f.failoverStatus = up

	for _, fn := range f.updaters {
		fn(f.handlerStatus || f.failoverStatus)
	}
}
