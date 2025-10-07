# s3_exporter

![Github Release Downloads](https://img.shields.io/github/downloads/linode-obs/s3_exporter/total.svg)
[![license](https://img.shields.io/badge/License-MIT-blue.svg)](https://github.com/linode-obs/s3_exporter/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/linode-obs/s3_exporter)]
[![contributions](https://img.shields.io/badge/contributions-welcome-brightgreen.svg?style=flat")](https://github.com/linode-obs/s3_exporter/issues)

**Production-safe S3 exporter for Prometheus with HEAD method support for efficient Ceph RGW monitoring.**

This exporter provides efficient bucket usage monitoring using Ceph RGW's HEAD method extension, eliminating the need for expensive ListObjects operations on large buckets. Originally forked from ribbybibby/s3_exporter with critical enhancements for production safety.

## Table of Contents
- [Installation](#installation)
- [Configuration for Linode Object Storage](#configuration-for-linode-object-storage)
- [CLI Reference](#cli-reference)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Metrics Instrumented](#metrics-instrumented)
- [Prometheus Configuration](#prometheus-configuration)
- [Troubleshooting Production Issues](#-troubleshooting-production-issues)
- [Why HEAD Method is Critical](#why-head-method-is-critical)
- [Contributing](#contributing)


## Installation

### Binary Release

Substitute `{{ version }}` for your desired release:

```bash
wget https://github.com/linode-obs/s3_exporter/releases/download/v{{ version }}/s3_exporter_{{ version }}_Linux_x86_64.tar.gz
tar xvf s3_exporter_{{ version }}_Linux_x86_64.tar.gz
./s3_exporter --version
```

### Docker

```bash
docker run -d \
  --name s3-exporter \
  -p 9340:9340 \
  -e AWS_ACCESS_KEY_ID="your-access-key" \
  -e AWS_SECRET_ACCESS_KEY="your-secret-key" \
  ghcr.io/linode-obs/s3_exporter:latest \
  --s3.endpoint-url=https://us-east-1.linodeobjects.com
```

### Building from Source

```bash
git clone https://github.com/linode-obs/s3_exporter.git
cd s3_exporter

# Using Make (recommended)
make build

# Or directly with Go
go build -v

# Run tests
make test

# Build Docker image
make docker
```

## Configuration for Linode Object Storage

### Environment Variables

```bash
# Linode Object Storage credentials (from Linode Cloud Manager)
export AWS_ACCESS_KEY_ID="your-linode-access-key"
export AWS_SECRET_ACCESS_KEY="your-linode-secret-key"

# Linode Object Storage endpoint (choose your region)
export S3_ENDPOINT_URL="https://us-east-1.linodeobjects.com"  # Newark
# export S3_ENDPOINT_URL="https://eu-central-1.linodeobjects.com"  # Frankfurt
# export S3_ENDPOINT_URL="https://ap-south-1.linodeobjects.com"  # Singapore

# Required for Linode Object Storage
export S3_FORCE_PATH_STYLE="true"
export S3_USE_HEAD_METHOD="true"
```

### Command Line Usage

```bash
# PRODUCTION SAFE (recommended)
./s3_exporter \
  --s3.endpoint-url=https://us-east-1.linodeobjects.com \
  --s3.force-path-style=true \
  --s3.use-head-method=true \
  --s3.prod-safe-mode=true \
  --web.listen-address=:9340

# The above flags are all DEFAULT values, so this is equivalent:
./s3_exporter --s3.endpoint-url=https://us-east-1.linodeobjects.com
```

### All Available Flags

```
  -h, --help                     Show context-sensitive help
      --web.listen-address=":9340"
                                 Address to listen on for web interface and telemetry
      --web.metrics-path="/metrics"
                                 Path under which to expose metrics
      --web.probe-path="/probe"  Path under which to expose the probe endpoint
      --web.discovery-path="/discovery"
                                 Path under which to expose service discovery
      --s3.endpoint-url=""       Custom endpoint URL (set to your Linode region)
      --s3.disable-ssl           Custom disable SSL (leave false for Linode)  
      --s3.force-path-style      Custom force path style (set to true for Linode)
      --s3.use-head-method=true  🛡️ Use HEAD method for Ceph RGW efficiency (DEFAULT)
      --s3.prod-safe-mode=true   🚨 Production safe mode - never use ListObjects (DEFAULT)
      --log.level="info"         Only log messages with the given severity or above
      --log.format="logger:stderr"
                                 Set the log target and format
      --version                  Show application version
```

Flags can also be set as environment variables, prefixed by `S3_EXPORTER_`. For example: `S3_EXPORTER_S3_ENDPOINT_URL=https://us-east-1.linodeobjects.com`.

## CLI Reference

### Critical Production Flags

| Flag | Default | Description | Production Use |
|------|---------|-------------|----------------|
| `--s3.prod-safe-mode` | `true` | 🚨 **CRITICAL**: Never fall back to ListObjects | ✅ **ALWAYS ENABLE** for production |
| `--s3.use-head-method` | `true` | ⚡ Use efficient HEAD requests with x-rgw-* headers | ✅ **REQUIRED** for large buckets |
| `--s3.endpoint-url` | `""` | Linode Object Storage endpoint URL | ✅ **REQUIRED**: Set to your region |
| `--s3.force-path-style` | `false` | Use path-style requests (required for Linode) | ✅ **REQUIRED**: Always `true` for Linode |

### Monitoring Endpoints

| Endpoint | Purpose | Example |
|----------|---------|---------|
| `/metrics` | Prometheus metrics | `curl http://localhost:9340/metrics` |
| `/probe` | Probe specific bucket | `curl http://localhost:9340/probe?bucket=my-bucket` |
| `/discovery` | Service discovery | `curl http://localhost:9340/discovery` |
| `/` | Health check | `curl http://localhost:9340/` |

### Docker Usage

```bash
docker build -t s3-exporter:latest .

docker run -d \
  --name s3-exporter \
  -p 9340:9340 \
  -e AWS_ACCESS_KEY_ID="your-access-key" \
  -e AWS_SECRET_ACCESS_KEY="your-secret-key" \
  s3-exporter:latest \
  --s3.endpoint-url=https://us-east-1.linodeobjects.com \
  --s3.force-path-style=true \
  --s3.prod-safe-mode=true \
  --s3.use-head-method=true
```

## Monitoring Loki Object Storage Usage

### Query Metrics for a Specific Bucket

```bash
curl "http://localhost:9340/probe?bucket=your-loki-bucket"
```

### Available Metrics

| Metric                             | Meaning                                                                                                     | Labels                    |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------- | ------------------------- |
| s3_biggest_object_size_bytes       | The size of the largest object.                                                                             | bucket, prefix            |
| s3_common_prefixes                 | A count of all the keys between the prefix and the next occurrence of the string specified by the delimiter | bucket, prefix, delimiter |
| s3_last_modified_object_date       | The modification date of the most recently modified object.                                                 | bucket, prefix            |
| s3_last_modified_object_size_bytes | The size of the object that was modified most recently.                                                     | bucket, prefix            |
| s3_list_duration_seconds           | The duration of the ListObjects operation                                                                   | bucket, prefix, delimiter |
| s3_list_success                    | Did the ListObjects operation complete successfully?                                                        | bucket, prefix, delimiter |
| s3_objects_size_sum_bytes          | The sum of the size of all the objects.                                                                     | bucket, prefix            |
| s3_objects                         | The total number of objects.                                                                                | bucket, prefix            |

### Key Metrics for Loki Monitoring

- `s3_objects{bucket="your-loki-bucket"}` - **Object count** (key for quota monitoring)
- `s3_objects_size_sum_bytes{bucket="your-loki-bucket"}` - Total storage used
- `s3_list_success{bucket="your-loki-bucket"}` - Monitoring health
- `s3_biggest_object_size_bytes{bucket="your-loki-bucket"}` - Largest object size
- `s3_last_modified_object_date{bucket="your-loki-bucket"}` - Most recent activity

### Common Prefixes Support

Instead of monitoring all objects in a bucket, you can use the `delimiter` parameter to count "folders" or prefixes:

```bash
curl 'http://localhost:9340/probe?bucket=your-loki-bucket&prefix=fake/&delimiter=/'
```

This is useful for monitoring Loki's internal structure without listing every individual chunk file.

## Metrics Instrumented

The following Prometheus metrics are exposed:

### Bucket Metrics (HEAD Method - Production Safe)

| Metric | Type | Description | Labels |
|--------|------|-------------|---------|
| `s3_bucket_objects_total` | Gauge | Total number of objects in bucket | `bucket`, `prefix` |
| `s3_bucket_size_bytes` | Gauge | Total size of bucket in bytes | `bucket`, `prefix` |
| `s3_list_success` | Gauge | Whether the bucket listing/HEAD was successful (1=success, 0=failure) | `bucket`, `prefix`, `delimiter` |
| `s3_list_duration_seconds` | Gauge | Time taken to retrieve bucket information | `bucket`, `prefix`, `delimiter` |

### Prefix/Delimiter Metrics (When Using ListObjects)

| Metric | Type | Description | Labels |
|--------|------|-------------|---------|
| `s3_common_prefixes` | Gauge | Number of common prefixes (when using delimiter) | `bucket`, `prefix`, `delimiter` |
| `s3_last_modified_object_date` | Gauge | Last modified date of the most recent object | `bucket`, `prefix` |
| `s3_last_modified_object_size_bytes` | Gauge | Size of the most recently modified object | `bucket`, `prefix` |
| `s3_biggest_object_size_bytes` | Gauge | Size of the largest object in bucket | `bucket`, `prefix` |

**Note**: The last 4 metrics are only available when using ListObjects (not recommended for production).

### Prometheus Configuration

#### Basic Configuration for Specific Buckets

Add this to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 's3-exporter-loki-bucket'
    static_configs:
      - targets: ['s3-exporter:9340']
    params:
      bucket: ['your-loki-bucket-name']
    metrics_path: /probe
    scrape_interval: 60s  # Adjust based on your needs
```

#### Advanced Configuration with Multiple Buckets

```yaml
scrape_configs:
  - job_name: "s3-linode"
    metrics_path: /probe
    static_configs:
      - targets:
          - bucket=loki-chunks;prefix=;
          - bucket=loki-ruler;prefix=;
          - bucket=backup-bucket;prefix=daily/;
    relabel_configs:
      - source_labels: [__address__]
        regex: "^bucket=(.*);prefix=(.*);$"
        replacement: "${1}"
        target_label: "__param_bucket"
      - source_labels: [__address__]
        regex: "^bucket=(.*);prefix=(.*);$"
        replacement: "${2}"
        target_label: "__param_prefix"
      - target_label: __address__
        replacement: s3-exporter:9340
```

#### Service Discovery (Automatic Bucket Detection)

For automatic discovery of all accessible buckets:

```yaml
scrape_configs:
  - job_name: "s3-linode-discovery"
    metrics_path: /probe
    http_sd_configs:
      - url: http://s3-exporter:9340/discovery
        refresh_interval: 300s
    relabel_configs:
      # Only monitor buckets starting with 'loki-' or 'backup-'
      - source_labels: [__param_bucket]
        action: keep
        regex: ^(loki-|backup-).*
```

### Alerting Rules

Create alerts for object count thresholds:

```yaml
groups:
  - name: loki-object-storage
    rules:
      - alert: LokiObjectCountHigh
        expr: s3_objects{bucket="your-loki-bucket"} > 100000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Loki bucket object count is high"
          description: "Loki bucket {{ $labels.bucket }} has {{ $value }} objects"
          
      - alert: LokiObjectCountCritical
        expr: s3_objects{bucket="your-loki-bucket"} > 500000
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Loki bucket object count is critical"
          description: "Loki bucket {{ $labels.bucket }} has {{ $value }} objects"

## Example Queries

### Prometheus Queries for Monitoring

```promql
# Object count growth rate (objects per hour)
rate(s3_objects[1h]) * 3600

# Buckets with no recent activity (more than 24 hours)
(time() - s3_last_modified_object_date) / 3600 > 24

# Bucket usage efficiency (average object size)
s3_objects_size_sum_bytes / s3_objects

# Buckets approaching object count limits
s3_objects > 50000

# Failed HEAD requests (falling back to ListObjects)
increase(s3_list_duration_seconds[5m]) > 0.1
```

### Grafana Dashboard Queries

```promql
# Total objects across all Loki buckets
sum by (bucket) (s3_objects{bucket=~"loki-.*"})

# Storage usage by bucket
sum by (bucket) (s3_objects_size_sum_bytes{bucket=~"loki-.*"}) / 1024^3

# Object count trend
increase(s3_objects[24h])
```

## Troubleshooting

### Common Issues

**HEAD Method Not Working:**
```bash
# Check if Ceph RGW headers are returned
curl -I https://us-east-1.linodeobjects.com/your-bucket
# Look for x-rgw-bytes-used and x-rgw-objects-count headers
```

**Authentication Issues:**
```bash
# Test credentials
aws s3 ls --endpoint-url=https://us-east-1.linodeobjects.com
```

**Connection Issues:**
```bash
# Test exporter health
curl http://localhost:9340/

# Test specific bucket
curl "http://localhost:9340/probe?bucket=your-bucket"
```

### Debugging

Enable debug logging:
```bash
./s3_exporter --log.level=debug
```

Check if HEAD method is being used:
```bash
# Look for log messages about HEAD method fallback
journalctl -u s3-exporter -f | grep "HEAD method"
```
```

## Kubernetes Deployment

See the `k8s/` directory for complete Kubernetes manifests including:

- `deployment.yaml` - Main application deployment
- `service.yaml` - Service definition
- `secret.yaml.template` - Credentials template
- `servicemonitor.yaml` - Prometheus ServiceMonitor

Update the deployment with your Linode Object Storage endpoint:

```yaml
env:
- name: S3_ENDPOINT_URL
  value: "https://us-east-1.linodeobjects.com"  # Your Linode region
```

## Why HEAD Method is Better

For large buckets (like Loki storage), the HEAD method provides:

1. **Speed**: Single request vs potentially thousands of ListObjects calls
2. **Efficiency**: No bandwidth wasted listing object details
3. **Scalability**: Performance doesn't degrade with object count
4. **Reliability**: Less likely to hit API rate limits

The HEAD method uses Ceph RGW's built-in bucket usage tracking via `x-rgw-bytes-used` and `x-rgw-objects-count` headers.

## 🚨 Troubleshooting Production Issues

### HEAD Method Failures

If you see errors like `HEAD method failed` in your logs:

```bash
# Check if your Ceph RGW supports HEAD method
curl -I -H "Authorization: AWS your-key:signature" \
  https://us-east-1.linodeobjects.com/your-bucket-name

# Look for x-rgw-* headers in the response
```

Expected headers for working HEAD method:
```
x-rgw-bytes-used: 1234567890
x-rgw-objects-count: 12345
```

### Production Safety Alerts

Monitor these log messages:

- ✅ `Using HEAD method for bucket usage` - Normal operation
- ⚠️ `HEAD method failed and prod-safe-mode enabled` - HEAD failed, no fallback (SAFE)
- 🚨 `HEAD method failed, falling back to ListObjects` - DANGEROUS for large buckets!

### Emergency Response

If your exporter is causing OBJ incidents:

```bash
# Immediately enable production safe mode
kubectl patch deployment s3-exporter -p '{"spec":{"template":{"spec":{"containers":[{"name":"s3-exporter","args":["--s3.endpoint-url=https://us-east-1.linodeobjects.com","--s3.force-path-style=true","--s3.prod-safe-mode=true"]}]}}}}'

# Or restart with safe flags
./s3_exporter --s3.prod-safe-mode=true --s3.use-head-method=true
```

## Linode Object Storage Regions

Update the endpoint URL based on your bucket's region:

- **Newark**: `https://us-east-1.linodeobjects.com`
- **Frankfurt**: `https://eu-central-1.linodeobjects.com` 
- **Singapore**: `https://ap-south-1.linodeobjects.com`

## Releasing

1. Merge commits to main
2. Tag release: `git tag -a v1.0.X -m "Release v1.0.X"`
3. Push tag: `git push origin v1.0.X`
4. GitHub Actions will automatically build and release

## Why HEAD Method is Critical

For production Loki buckets, the HEAD method provides:

- **🚀 Speed**: Single request vs potentially thousands of ListObjects calls
- **💾 Efficiency**: No bandwidth wasted listing object details  
- **📈 Scalability**: Performance doesn't degrade with object count
- **🛡️ Safety**: Cannot cause storage backend performance issues or OBJ incidents

The HEAD method uses Ceph RGW's built-in bucket usage tracking via `x-rgw-bytes-used` and `x-rgw-objects-count` headers.

## Contributing

Contributions welcome! This project follows standard Go project practices:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass: `make test`
6. Submit a pull request

## Original Project

This is a fork of [ribbybibby/s3_exporter](https://github.com/ribbybibby/s3_exporter) enhanced for Linode Object Storage and Ceph RGW optimization with critical production safety improvements.