# Prometheus Integration

This document describes how ktop collects metrics directly from Kubernetes components using Prometheus-format endpoints.

## Overview

Kubernetes components (kubelet, cAdvisor, API server, etc.) expose metrics in Prometheus format. ktop can scrape these endpoints directly, providing richer metrics than the Kubernetes Metrics Server alone—including network I/O, disk usage, and per-container resource breakdowns.

This approach works without deploying a full Prometheus stack. ktop handles scraping, storage, and visualization in a single binary.

## When to Use Prometheus Mode

| Use Prometheus | Use Metrics Server |
|----------------|-------------------|
| Self-managed clusters (minikube, kind, k3s, bare metal) | Managed Kubernetes (GKE, EKS, AKS) |
| Need network/disk I/O metrics | Basic CPU/memory is sufficient |
| Need per-container breakdowns | Want lower memory footprint |
| Want historical data for sparklines | Metrics Server already deployed |

## Quick Start

```bash
# Basic usage with defaults
ktop --metrics-source=prometheus

# Custom configuration
ktop --metrics-source=prometheus \
     --prometheus-scrape-interval=30s \
     --prometheus-retention=2h
```

Default settings: 5s scrape interval, 1h retention, 10,000 max samples, kubelet+cadvisor components.

## Architecture

```
┌───────────────────────────────────────────────────┐
│              Kubernetes Cluster                    │
│  ┌──────────────────┐  ┌──────────────────┐       │
│  │     kubelet      │  │     cAdvisor     │       │
│  │    /metrics      │  │ /metrics/cadvisor│       │
│  └────────┬─────────┘  └────────┬─────────┘       │
└───────────┼─────────────────────┼─────────────────┘
            │                     │
            └──────────┬──────────┘
                       │
              ┌────────▼────────┐
              │    Scraper      │  prom/scraper.go
              │ (discovery +    │  - discovers endpoints
              │  HTTP fetch)    │  - parses Prometheus format
              └────────┬────────┘
                       │
              ┌────────▼────────┐
              │    Storage      │  prom/storage.go
              │ (in-memory      │  - ring buffers per series
              │  time series)   │  - retention management
              └────────┬────────┘
                       │
              ┌────────▼────────┐
              │PromMetricsSource│  metrics/prom/prom_source.go
              │ (ktop adapter)  │  - rate calculations
              └────────┬────────┘  - aggregations
                       │
              ┌────────▼────────┐
              │   ktop Views    │
              │  (UI display)   │
              └─────────────────┘
```

### How Scraping Works

ktop accesses metrics **through the Kubernetes API proxy**, not by directly connecting to node IPs:

- You don't need direct network access to node IPs
- No VPN or bastion hosts required
- Works from anywhere you can run `kubectl`

The API server proxies requests:
- **Kubelet**: `/api/v1/nodes/{node}/proxy/metrics`
- **cAdvisor**: `/api/v1/nodes/{node}/proxy/metrics/cadvisor`

## Components

ktop currently scrapes metrics from the following Kubernetes components:

| Component | Port | Metrics Provided |
|-----------|------|------------------|
| `kubelet` | 10250 | Node CPU, memory, pod counts |
| `cadvisor` | 10250 | Container CPU, memory, network I/O, disk I/O |

These components provide comprehensive node and container-level metrics for monitoring cluster health. Additional Kubernetes control plane and workload components will be added in future releases to provide deeper cluster visibility.

## Metrics Collected

### Node Metrics (from cAdvisor root container)

| Metric | Source | Description |
|--------|--------|-------------|
| CPU usage | `container_cpu_usage_seconds_total` | Total CPU time (converted to rate) |
| Memory | `container_memory_working_set_bytes` | Working set memory |
| Network RX | `container_network_receive_bytes_total` | Bytes received (rate) |
| Network TX | `container_network_transmit_bytes_total` | Bytes transmitted (rate) |
| Disk Read | `container_fs_reads_bytes_total` | Disk read bytes (rate) |
| Disk Write | `container_fs_writes_bytes_total` | Disk write bytes (rate) |

### Pod/Container Metrics

| Metric | Source | Description |
|--------|--------|-------------|
| CPU usage | `container_cpu_usage_seconds_total` | Per-container CPU (rate) |
| Memory | `container_memory_working_set_bytes` | Per-container memory |

Metrics are filtered to exclude the `POD` pause container and aggregated per-pod when needed.

## Configuration

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--metrics-source` | `prometheus` | Source: `prometheus`, `metrics-server`, or `none` |
| `--prometheus-scrape-interval` | `5s` | How often to scrape (min: 5s) |
| `--prometheus-retention` | `1h` | How long to keep metrics (min: 5m) |
| `--prometheus-max-samples` | `10000` | Max samples per time series |
| `--prometheus-components` | `kubelet,cadvisor` | Components to scrape |

## RBAC Requirements

Prometheus mode requires permissions to access node proxy endpoints:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ktop-prometheus
rules:
- apiGroups: [""]
  resources: ["nodes/proxy"]
  verbs: ["get"]
- apiGroups: [""]
  resources: ["nodes", "pods"]
  verbs: ["get", "list", "watch"]
```

Apply the provided manifest:
```bash
kubectl apply -f https://raw.githubusercontent.com/vladimirvivien/ktop/main/hack/deploy/rbac-prometheus.yaml
```

Check your permissions:
```bash
kubectl auth can-i get nodes/proxy
kubectl auth can-i get pods/proxy
kubectl auth can-i get --raw /metrics
```

## Troubleshooting

### "failed to start prometheus collection"

**Cause**: Insufficient RBAC permissions or network issues.

**Solutions**:
1. Check permissions: `kubectl auth can-i get nodes/proxy`
2. Apply RBAC manifest (see above)
3. Try fewer components: `--prometheus-components=kubelet`
4. Fall back: `--metrics-source=metrics-server`

### "metrics: not connected" in header

**Cause**: Scraping hasn't succeeded yet.

**Solutions**:
1. Wait 15-30 seconds for initial scrape
2. Verify cluster supports endpoint access (not GKE/EKS/AKS)
3. Test manually:
   ```bash
   kubectl proxy &
   curl http://localhost:8001/api/v1/nodes/<node-name>/proxy/metrics
   ```

### Tuning Memory Usage

In-memory storage scales with active nodes, active containers, and the retention window (detail sparklines query the trailing 5 minutes). On large clusters, high-churn environments (frequent rollouts, Jobs, or CronJobs), or memory-constrained jump hosts, tune the collector with CLI flags or `~/.ktop/config.yaml`:

```bash
# Recommended low-memory profile for large or high-churn clusters
ktop --prometheus-scrape-interval=15s \
     --prometheus-retention=10m \
     --prometheus-max-samples=300
```

You can persist these defaults in `~/.ktop/config.yaml` (CLI flags always take precedence):

```yaml
metrics:
  source: prometheus
  scrape_interval: 15s
  retention: 10m
  max_samples: 300
  ksm_mode: "off"
```

Additional levers for reducing memory footprint:

1. **Shorten retention (`--prometheus-retention=10m`, min `5m`)**: Evicts time series from deleted pods and completed jobs sooner instead of keeping them for an hour.
2. **Increase scrape interval (`--prometheus-scrape-interval=15s` or `30s`)**: Reduces API-server `nodes/proxy` traffic and scrape-time allocation pressure on multi-node clusters. cAdvisor updates on a 10–15s housekeeping cycle, so `15s` preserves responsiveness.
3. **Cap per-series samples (`--prometheus-max-samples=300`, min `100`)**: Bounds the maximum samples retained per time series.
4. **Disable optional enrichment (`--ksm=off`)**: Skips `kube-state-metrics` discovery and scraping when governance badges are not needed.
5. **Scope to a namespace (`-n <namespace>`)**: Limits Kubernetes informer caches to a single namespace when cluster-wide pod visibility is unnecessary.
6. **Use Metrics Server on small VMs (`--metrics-source=metrics-server`)**: Avoids kubelet/cAdvisor scraping when only CPU and memory usage are needed.
7. **Set `GOMEMLIMIT` in constrained environments**: Running with `GOMEMLIMIT=256MiB ktop ...` instructs the Go garbage collector to collect more aggressively and return unused heap memory to the OS sooner.

## Limitations

1. **Managed Kubernetes**: GKE, EKS, AKS restrict control plane access. Prometheus mode may not work.

2. **Memory-only storage**: Metrics are lost when ktop exits. No disk persistence.

3. **No PromQL**: Simple label-based queries only.

4. **Single process**: Not designed for distributed deployment.

## Fallback Behavior

When `--metrics-source` is not specified:
1. ktop tries Prometheus mode first
2. If RBAC denies access, falls back to Metrics Server
3. If Metrics Server is unavailable, shows resource requests/limits only

When `--metrics-source=prometheus` is explicitly set:
- Fails with error if Prometheus scraping doesn't work
- No automatic fallback (explicit user choice)

## Package Structure

For developers interested in the implementation:

| File | Purpose |
|------|---------|
| `prom/types.go` | Core types: `MetricSample`, `TimeSeries`, `ScrapeTarget` |
| `prom/scraper.go` | `KubernetesScraper` - endpoint discovery and HTTP scraping |
| `prom/storage.go` | `InMemoryStore` - time-series storage with label indexing |
| `prom/ring_buffer.go` | Generic circular buffer for efficient sample storage |
| `prom/controller.go` | `CollectorController` - orchestrates scraping and cleanup |
| `metrics/prom/prom_source.go` | `PromMetricsSource` - implements ktop's `MetricsSource` interface |
