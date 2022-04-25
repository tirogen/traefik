---
title: "Traefik OpenTelemetry Documentation"
description: "Traefik supports several tracing backends, including OpenTelemetry. Learn how to implement it for observability in Traefik Proxy. Read the technical documentation."
---

# OpenTelemetry

To enable the OpenTelemetry tracer:

```yaml tab="File (YAML)"
tracing:
  openTelemetry: {}
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry]
```

```bash tab="CLI"
--tracing.openTelemetry=true
```:wq


#### `endpoint`

_Required, Default="localhost:4318"_

This instructs the reporter to send spans to the OpenTelemetry Collector at this address (host:port).

```yaml tab="File (YAML)"
tracing:
  openTelemetry:
    endpoint: localhost:4318
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry]
    endpoint = "localhost:4318"
```

```bash tab="CLI"
--tracing.openTelemetry.endpoint=localhost:4318
```

#### `urlPath`

_Optional, Default="/v1/traces"_

Override the default URL path used for sending traces.

```yaml tab="File (YAML)"
tracing:
  openTelemetry:
    urlPath: /v1/traces
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry]
    urlPath = "/v1/traces"
```

```bash tab="CLI"
--tracing.openTelemetry.urlPath="/v1/traces"
```

#### `insecure`

_Optional, Default=false_

Allows reporter to send span to the OpenTelemetry Collector using the HTTP protocol.

```yaml tab="File (YAML)"
tracing:
  openTelemetry:
    insecure: true
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry]
    insecure = true
```

```bash tab="CLI"
--tracing.openTelemetry.insecure=true
```

#### `compress`

_Optional, Default=false_

Allows reporter to send span to the OpenTelemetry Collector using gzip compression.

```yaml tab="File (YAML)"
tracing:
  openTelemetry:
    compress: true
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry]
    compress = true
```

```bash tab="CLI"
--tracing.openTelemetry.compress=true
```

#### `timeout`

_Optional, Default="10s"_

The max waiting time for the backend to process each spans batch.

```yaml tab="File (YAML)"
tracing:
  openTelemetry:
    timeout: 3s
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry]
    timeout = "3s"
```

```bash tab="CLI"
--tracing.openTelemetry.timeout=3s
```

#### `headers`

_Optional, Default={}_

Additional headers sent with spans by the reporter to the OpenTelemetry Collector.

```yaml tab="File (YAML)"
tracing:
  openTelemetry:
    headers:
      foo: bar
      baz: buz
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry.headers]
    foo = bar
    baz = buz
```

```bash tab="CLI"
--tracing.openTelemetry.headers.foo=bar --tracing.openTelemetry.headers.baz=buz
```

#### `retry`

_Optional_

Enable retries when the reporter sends span to the OpenTelemetry Collector.

```yaml tab="File (YAML)"
tracing:
  openTelemetry:
    retry: {}
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry.retry]
```

```bash tab="CLI"
--tracing.openTelemetry.retry=true
```

##### `initialInterval`

_Optional, Default=5s_

The time to wait after the first failure before retrying.

```yaml tab="File (YAML)"
tracing:
  openTelemetry:
    retry:
      initialInterval: 10s
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry.retry]
    initialInterval = "10s"
```

```bash tab="CLI"
--tracing.openTelemetry.retry.initialInterval=10s
```

##### `maxInterval`

_Optional, Default=30s_

The upper bound on backoff interval.

```yaml tab="File (YAML)"
tracing:
  openTelemetry:
    retry:
      maxInterval: 10s
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry.retry]
    maxInterval = "10s"
```

```bash tab="CLI"
--tracing.openTelemetry.retry.maxInterval=10s
```

##### `maxElapsedTime`

_Optional, Default=1m_

The maximum amount of time (including retries) spent trying to send a request/batch.

```yaml tab="File (YAML)"
tracing:
  openTelemetry:
    retry:
      maxElapsedTime: 10s
```

```toml tab="File (TOML)"
[tracing]
  [tracing.openTelemetry.retry]
    maxElapsedTime = "10s"
```

```bash tab="CLI"
--tracing.openTelemetry.retry.maxElapsedTime=10s
```
