package metrics

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/generic"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	iLog "github.com/influxdata/influxdb-client-go/v2/log"
	"github.com/traefik/traefik/v2/pkg/log"
	"github.com/traefik/traefik/v2/pkg/types"
)

var (
	influxDB2Client   influxdb2.Client
	influxDB2WriteAPI api.WriteAPI
)

// RegisterInfluxDB2 creates metrics exporter for InfluxDB2.
func RegisterInfluxDB2(ctx context.Context, config *types.InfluxDB2) Registry {
	iLog.Log = nil // Disable influxDB2 internal logs
	if influxDB2Client == nil {
		flushMs := uint(time.Duration(config.PushInterval).Milliseconds())
		options := influxdb2.DefaultOptions()
		options = options.SetBatchSize(config.BatchSize)
		options = options.SetFlushInterval(flushMs)
		influxDB2Client = influxdb2.NewClientWithOptions(config.Address, config.Token, options)
		if influxDB2Client == nil { // FIXME seems that it does never happen
			log.FromContext(ctx).Error("Failed to connect to InfluxDB v2")
			return nil
		}

		influxDB2WriteAPI = influxDB2Client.WriteAPI(config.Org, config.Bucket)
		if influxDB2WriteAPI == nil { // FIXME seems that it does never happen
			log.FromContext(ctx).Error("Failed to open InfluxDB v2 bucket")
			influxDB2Client.Close()
			influxDB2Client = nil
		}

		go func() {
			for {
				select {
				case err := <-influxDB2WriteAPI.Errors():
					log.FromContext(ctx).Errorf("%+v", err)
				}
			}
		}()
	}

	registry := &standardRegistry{
		configReloadsCounter:           newInfluxDB2Counter(configReloadsTotalName),
		configReloadsFailureCounter:    newInfluxDB2Counter(configReloadsFailuresTotalName),
		lastConfigReloadSuccessGauge:   newInfluxDB2Gauge(configLastReloadSuccessName),
		lastConfigReloadFailureGauge:   newInfluxDB2Gauge(configLastReloadFailureName),
		tlsCertsNotAfterTimestampGauge: newInfluxDB2Gauge(tlsCertsNotAfterTimestamp),
	}

	if config.AddEntryPointsLabels {
		registry.epEnabled = config.AddEntryPointsLabels
		registry.entryPointReqsCounter = newInfluxDB2Counter(entryPointReqsTotalName)
		registry.entryPointReqsTLSCounter = newInfluxDB2Counter(entryPointReqsTLSTotalName)
		registry.entryPointReqDurationHistogram, _ = NewHistogramWithScale(newInfluxDB2Histogram(entryPointReqDurationName), time.Second)
		registry.entryPointOpenConnsGauge = newInfluxDB2Gauge(entryPointOpenConnsName)
	}

	if config.AddRoutersLabels {
		registry.routerEnabled = config.AddRoutersLabels
		registry.routerReqsCounter = newInfluxDB2Counter(routerReqsTotalName)
		registry.routerReqsTLSCounter = newInfluxDB2Counter(routerReqsTLSTotalName)
		registry.routerReqDurationHistogram, _ = NewHistogramWithScale(newInfluxDB2Histogram(routerReqDurationName), time.Second)
		registry.routerOpenConnsGauge = newInfluxDB2Gauge(routerOpenConnsName)
	}

	if config.AddServicesLabels {
		registry.svcEnabled = config.AddServicesLabels
		registry.serviceReqsCounter = newInfluxDB2Counter(serviceReqsTotalName)
		registry.serviceReqsTLSCounter = newInfluxDB2Counter(serviceReqsTLSTotalName)
		registry.serviceReqDurationHistogram, _ = NewHistogramWithScale(newInfluxDB2Histogram(serviceReqDurationName), time.Second)
		registry.serviceRetriesCounter = newInfluxDB2Counter(serviceRetriesTotalName)
		registry.serviceOpenConnsGauge = newInfluxDB2Gauge(serviceOpenConnsName)
		registry.serviceServerUpGauge = newInfluxDB2Gauge(serviceServerUpName)
	}

	return registry
}

// StopInfluxDB2 flushes and removes InfluxDB2 client and WriteAPI.
func StopInfluxDB2() {
	if influxDB2Client != nil {
		influxDB2Client.Close()
	}
	influxDB2Client = nil

	if influxDB2WriteAPI != nil {
		influxDB2WriteAPI.Flush()
	}
	influxDB2WriteAPI = nil
}

func sendInfluxDB2(name string, labels []string, value interface{}) {
	tags := make(map[string]string)
	fields := make(map[string]interface{})
	for i := range labels {
		if i%2 != 0 {
			continue
		} else if i+1 >= len(labels) {
			break
		}
		tags[labels[i]] = labels[i+1]
	}

	fields[name] = value

	p := influxdb2.NewPoint("traefik", tags, fields, time.Now())
	influxDB2WriteAPI.WritePoint(p)
}

type influxDB2Counter struct {
	c        *generic.Counter
	counters *sync.Map
}

func newInfluxDB2Counter(name string) *influxDB2Counter {
	return &influxDB2Counter{
		c:        generic.NewCounter(name),
		counters: &sync.Map{},
	}
}

// With returns a new influxDB2Counter with the given labels.
func (c *influxDB2Counter) With(labels ...string) metrics.Counter {
	newCounter := c.c.With(labels...).(*generic.Counter)
	newCounter.ValueReset()

	return &influxDB2Counter{
		c:        newCounter,
		counters: c.counters,
	}
}

// Add adds the given delta to the counter.
func (c *influxDB2Counter) Add(delta float64) {
	labelsKey := strings.Join(c.c.LabelValues(), ",")
	v, _ := c.counters.LoadOrStore(labelsKey, c)
	counter := v.(*influxDB2Counter)
	counter.c.Add(delta)

	sendInfluxDB2(counter.c.Name, counter.c.LabelValues(), counter.c.Value())
}

type influxDB2Gauge struct {
	g      *generic.Gauge
	gauges *sync.Map
}

func newInfluxDB2Gauge(name string) *influxDB2Gauge {
	return &influxDB2Gauge{
		g:      generic.NewGauge(name),
		gauges: &sync.Map{},
	}
}

// With returns a new pilotGauge with the given labels.
func (g *influxDB2Gauge) With(labels ...string) metrics.Gauge {
	newGauge := g.g.With(labels...).(*generic.Gauge)
	newGauge.Set(0)

	return &influxDB2Gauge{
		g:      newGauge,
		gauges: g.gauges,
	}
}

// Set sets the given value to the gauge.
func (g *influxDB2Gauge) Set(value float64) {
	labelsKey := strings.Join(g.g.LabelValues(), ",")
	v, _ := g.gauges.LoadOrStore(labelsKey, g)
	gauge := v.(*influxDB2Gauge)
	gauge.g.Set(value)

	sendInfluxDB2(gauge.g.Name, gauge.g.LabelValues(), value)
}

// Add adds the given delta to the gauge.
func (g *influxDB2Gauge) Add(delta float64) {
	labelsKey := strings.Join(g.g.LabelValues(), ",")
	v, _ := g.gauges.LoadOrStore(labelsKey, g)
	gauge := v.(*influxDB2Gauge)
	gauge.g.Add(delta)

	sendInfluxDB2(gauge.g.Name, gauge.g.LabelValues(), gauge.g.Value())
}

type influxDB2Histogram struct {
	g *generic.Gauge
}

func newInfluxDB2Histogram(name string) *influxDB2Histogram {
	return &influxDB2Histogram{
		g: generic.NewGauge(name),
	}
}

// With returns a new influxDB2Histogram with the given labels.
func (h *influxDB2Histogram) With(labels ...string) metrics.Histogram {
	newGauge := h.g.With(labels...).(*generic.Gauge)
	newGauge.Set(0)

	return &influxDB2Histogram{
		g: newGauge,
	}
}

// Observe records a new value into the histogram.
func (h *influxDB2Histogram) Observe(value float64) {
	h.g.Set(value)
	sendInfluxDB2(h.g.Name, h.g.LabelValues(), value)
}
