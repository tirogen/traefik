---
title: "Traefik OpenTelemetry Documentation"
description: "Traefik supports several metrics backends, including OpenTelemetry. Learn how to implement it for observability in Traefik Proxy. Read the technical documentation."
---

# OpenTelemetry

To enable the OpenTelemetry:

```yaml tab="File (YAML)"
metrics:
  openTelemetry: {}
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
```

```bash tab="CLI"
--metrics.openTelemetry=true
```

#### `addEntryPointsLabels`

_Optional, Default=true_

Enable metrics on entry points.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    addEntryPointsLabels: true
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    addEntryPointsLabels = true
```

```bash tab="CLI"
--metrics.openTelemetry.addEntryPointsLabels=true
```

#### `addRoutersLabels`

_Optional, Default=false_

Enable metrics on routers.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    addRoutersLabels: true
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    addRoutersLabels = true
```

```bash tab="CLI"
--metrics.openTelemetry.addrouterslabels=true
```

#### `addServicesLabels`

_Optional, Default=true_

Enable metrics on services.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    addServicesLabels: true
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    addServicesLabels = true
```

```bash tab="CLI"
--metrics.openTelemetry.addServicesLabels=true
```

#### `buckets`

_Optional, Default=".005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10"_

Explicit boundaries for Histogram data points.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    explicitBoundaries:
      - 0.1
      - 0.3
      - 1.2
      - 5.0
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    explicitBoundaries = [0.1,0.3,1.2,5.0]
```

```bash tab="CLI"
--metrics.openTelemetry.explicitBoundaries=0.1,0.3,1.2,5.0
```

#### `withMemory`

_Optional, Default=false_

Controls whether the processor remembers metric instruments and label sets that were previously reported.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    withMemory: true
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    withMemory = true
```

```bash tab="CLI"
--metrics.openTelemetry.withMemory=true
```

#### `compress`

_Optional, Default=false_

Allows reporter to send metrics to the OpenTelemetry Collector using gzip compression.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    compress: true
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    compress = true
```

```bash tab="CLI"
--metrics.openTelemetry.compress=true
```

#### `endpoint`

_Required, Default="localhost:4318"_

Endpoint instructs exporter to send metrics to influxdb at this address.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    endpoint: localhost:8089
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    endpoint = "localhost:8089"
```

```bash tab="CLI"
--metrics.openTelemetry.endpoint=localhost:8089
```

#### `headers`

_Optional, Default={}_

Additional headers sent with metrics by the reporter to the OpenTelemetry Collector.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    headers:
      foo: bar
      baz: buz
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry.headers]
    foo = bar
    baz = buz
```

```bash tab="CLI"
--metrics.openTelemetry.headers.foo=bar --metrics.openTelemetry.headers.baz=buz
```

#### `insecure`

_Optional, Default=false_

Allows reporter to send span to the OpenTelemetry Collector without using a secured protocol.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    insecure: true
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    insecure = true
```

```bash tab="CLI"
--metrics.openTelemetry.insecure=true
```

#### `pushInterval`

_Optional, Default=10s_

The interval used by the exporter to push metrics to OpenTelemetry.
The interval value must be greater than zero.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    pushInterval: 10s
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    pushInterval = "10s"
```

```bash tab="CLI"
--metrics.openTelemetry.pushInterval=10s
```

#### `pushTimeout`

_Optional, Default=10s_

Timeout defines how long to wait on an idle session before releasing the related resources
when pushing metrics to OpenTelemetry.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    pushTimeout: 10s
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    pushTimeout = "10s"
```

```bash tab="CLI"
--metrics.openTelemetry.pushTimeout=10s
```

#### `retry`

_Optional_

Enable retries when the reporter sends metrics to the OpenTelemetry Collector.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    retry: {}
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry.retry]
```

```bash tab="CLI"
--metrics.openTelemetry.retry=true
```

##### `initialInterval`

_Optional, Default=5s_

The time to wait after the first failure before retrying.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    retry:
      initialInterval: 10s
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry.retry]
    initialInterval = "10s"
```

```bash tab="CLI"
--metrics.openTelemetry.retry.initialInterval=10s
```

##### `maxInterval`

_Optional, Default=30s_

The upper bound on backoff interval.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    retry:
      maxInterval: 10s
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry.retry]
    maxInterval = "10s"
```

```bash tab="CLI"
--metrics.openTelemetry.retry.maxInterval=10s
```

##### `maxElapsedTime`

_Optional, Default=1m_

The maximum amount of time (including retries) spent trying to send a request/batch.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    retry:
      maxElapsedTime: 10s
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry.retry]
    maxElapsedTime = "10s"
```

```bash tab="CLI"
--metrics.openTelemetry.retry.maxElapsedTime=10s
```

#### `timeout`

_Optional, Default="10s"_

The max waiting time for the backend to process each metrics batch.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    timeout: 3s
```

```toml tab="File (TOML)"
[metrics]
  [metric.openTelemetry]
    timeout = "3s"
```

```bash tab="CLI"
--metrics.openTelemetry.timeout=3s
```

#### HTTP configuration

This instructs the reporter to send metrics to the OpenTelemetry Collector using HTTP:

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    http: {}
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry.http]
```

```bash tab="CLI"
--metrics.openTelemetry.http=true
```

##### `urlPath`

_Optional, Default="/v1/metrics"_

Override the default URL path used for sending metrics.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    http:
      urlPath: /v1/metrics
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    [metrics.openTelemetry.http]
      urlPath = "/v1/metrics"
```

```bash tab="CLI"
--metrics.openTelemetry.http.urlPath="/v1/metrics"
```

#### GRPC configuration

This instructs the reporter to send metrics to the OpenTelemetry Collector using GRPC:

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    grpc: {}
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry.grpc]
```

```bash tab="CLI"
--metrics.openTelemetry.grpc=true
```

##### `reconnectionPeriod`

_Optional_

The minimum amount of time between connection attempts to the target endpoint.

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    grpc:
      reconnectionPeriod: 30s
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    [metrics.openTelemetry.grpc]
      reconnectionPeriod = "30s"
```

```bash tab="CLI"
--metrics.openTelemetry.grpc.reconnectionPeriod=30s
```

##### `serviceConfig`

_Optional_

Defines the JSON representation of the default gRPC service config used.

For more information about service configurations, see: https://github.com/grpc/grpc/blob/master/doc/service_config.md

```yaml tab="File (YAML)"
metrics:
  openTelemetry:
    grpc:
      serviceConfig: {}
```

```toml tab="File (TOML)"
[metrics]
  [metrics.openTelemetry]
    [metrics.openTelemetry.grpc]
      serviceConfig = "{}"
```

```bash tab="CLI"
--metrics.openTelemetry.grpc.serviceConfig={}
```