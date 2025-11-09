# Prometheus Integration Design Document

## Overview

This document describes the design and integration of Prometheus-based metrics collection into ktop. The goal is to expand the available metrics beyond what the Kubernetes Metrics Server provides while maintaining ktop's focus as a lightweight, terminal-based monitoring tool.

## Motivation

ktop currently relies on the Kubernetes Metrics Server for resource metrics. This works, but has limitations:

1. **Limited metrics**: Only CPU and memory usage are available. We're missing network I/O, disk usage, load averages, and detailed container metrics.
2. **Single source dependency**: If Metrics Server is unavailable, ktop has no metrics at all.
3. **No historical data**: Can't show trends or track changes over time.
4. **Missing container details**: No visibility into throttling, limits, or per-container resource usage.

Kubernetes components already expose Prometheus metrics endpoints (kubelet, cAdvisor, API server, etcd). By scraping these directly, we can access significantly more metrics without requiring a full Prometheus deployment.

## Goals

1. **Expand available metrics**: Expose network I/O, load averages, disk usage, container counts, throttling data, and other metrics already available from Kubernetes components.

2. **Improve reliability**: Support multiple metrics sources with automatic fallback. If Prometheus scraping fails, fall back to Metrics Server.

3. **Maintain backward compatibility**: Existing behavior unchanged by default. Enhanced features are opt-in via flags.

4. **Keep it lightweight**: Efficient in-memory storage with configurable retention. No disk persistence, no heavy processing.

5. **Add basic trend analysis**: Show whether resource usage is trending up, down, or stable over a recent time window.

6. **Support flexible configuration**: Multiple modes (prometheus-only, metrics-server-only, hybrid, auto), configurable scrape intervals and retention.

## Non-Goals

1. **Not a full Prometheus replacement**: No long-term storage, no complex queries, no alerting.

2. **No disk persistence**: Metrics stored in memory only with short retention windows (1-6 hours typical).

3. **No custom metrics**: Focus on standard Kubernetes component metrics. Application-specific custom metrics are out of scope.

4. **No distributed deployment**: Remains a single-binary CLI tool.

5. **No query language**: Simple key-value metric lookups only, no PromQL support.

6. **No alerting**: ktop is for visualization, not alerting.

7. **No plugin system**: No third-party integrations or extensibility mechanisms.

## Architecture

### Current Architecture

```
Kubernetes Metrics Server → k8s/metrics_controller.go → views/model/*.go → TUI Display
```

### Enhanced Architecture

```
Kubernetes Components → prom/scraper.go → prom/storage.go → Adapter Layer → Enhanced Models → TUI Display
                    ↓
         Metrics Server (fallback)
```

### Components

#### Metrics Source Interface

Abstraction over different metrics sources:

```go
type MetricsSource interface {
    GetNodeMetrics(ctx context.Context, nodeName string) (*NodeMetrics, error)
    GetPodMetrics(ctx context.Context, namespace, podName string) (*PodMetrics, error)
    GetAllPodMetrics(ctx context.Context) ([]*PodMetrics, error)
    GetAvailableMetrics() []string
    IsHealthy() bool
    GetSourceInfo() SourceInfo
}
```

This allows switching between Prometheus and Metrics Server transparently.

#### Prometheus Metrics Source

Implements `MetricsSource` using the prom package:

```go
type PromMetricsSource struct {
    controller *prom.CollectorController
    store      prom.MetricsStore
    config     *PromConfig
    healthy    bool
    mu         sync.RWMutex
}
```

Scrapes Kubernetes component endpoints and maps metrics to ktop's data model:
- `kubelet_node_cpu_usage_seconds_total` → Node CPU usage
- `kubelet_node_memory_working_set_bytes` → Node memory usage
- `kubelet_node_network_receive_bytes_total` → Network RX
- `container_cpu_usage_seconds_total` → Container CPU
- `kubelet_running_pods` → Pod count

#### Metrics Source Selection

Simple runtime selection between two mutually exclusive sources:

```go
type MetricsController struct {
    source MetricsSource
    config *MetricsConfig
}
```

Supports two modes:
- **metrics-server**: Default mode, uses Kubernetes Metrics Server (backward compatible)
  - **Graceful fallback**: If Metrics Server is unavailable, automatically falls back to resource requests/limits from pod specs
  - No errors shown to user - works with best available data
  - Maintains existing ktop behavior where it works even without Metrics Server installed
- **prometheus**: Uses Prometheus scraping from Kubernetes components
  - Explicit opt-in for enhanced metrics
  - Requires RBAC permissions to access component endpoints
  - If unavailable, shows clear error (no fallback)
  - User makes explicit choice, gets explicit feedback

**No hybrid mode** - users explicitly choose one source at startup.

#### Enhanced Data Models

Extended models include additional fields:

```go
type EnhancedNodeModel struct {
    *NodeModel // Existing fields

    // Additional Prometheus metrics
    NetworkRxBytes     *resource.Quantity
    NetworkTxBytes     *resource.Quantity
    DiskUsage          *resource.Quantity
    LoadAverage1m      float64
    LoadAverage5m      float64
    LoadAverage15m     float64
    ContainerCount     int
    RunningPodCount    int

    // Metadata
    MetricsSource      string
    LastUpdate         time.Time
    SourceHealthy      bool

    // Trends
    CPUTrend           TrendIndicator
    MemoryTrend        TrendIndicator
}
```

#### Enhanced UI

New columns when `--enhanced-columns` is enabled:

| Column | Description | Source |
|--------|-------------|--------|
| LOAD | Load average (1m/5m/15m) | `kubelet_node_load*` |
| NETWORK | Network I/O (Rx/Tx) | `kubelet_node_network_*_bytes_total` |
| CONTAINERS | Running containers | `container_count` |
| HEALTH | Metrics source health | Health monitoring |
| TREND | Resource usage trend | Historical comparison |

Visual indicators:
- Source: `P` (Prometheus), `M` (Metrics Server)
- Trends: `↗` (up), `↘` (down), `→` (flat)

## Configuration

### YAML Configuration

```yaml
source:
  type: metrics-server    # metrics-server | prometheus

prometheus:
  enabled: true
  scrape_interval: 15s
  retention_time: 1h
  max_samples: 10000
  components:
    - kubelet
    - cadvisor
    - apiserver

display:
  enhanced_columns: false
  show_trends: false
  show_health: true
  time_range: 15m
  refresh_interval: 2s

advanced:
  debug: false
  log_level: info
  max_concurrency: 10
  cache_size: 1000
```

### CLI Flags

Metrics source:
```bash
--metrics-source=metrics-server           # Source selection: metrics-server (default) | prometheus
--prometheus-scrape-interval=15s          # Scrape interval (when using prometheus)
--prometheus-retention=1h                 # Retention time (when using prometheus)
--prometheus-components=kubelet,cadvisor  # Components to scrape (when using prometheus)
```

Display:
```bash
--enhanced-columns      # Show additional columns
--show-trends          # Display trend indicators
--show-health          # Show source health
--time-range=15m       # Trend calculation window
```

## Usage Examples

### Default (Metrics Server)

```bash
ktop
# or explicitly:
ktop --metrics-source=metrics-server
```

Uses Kubernetes Metrics Server with graceful fallback:
- **If Metrics Server is available:** Shows real-time CPU and memory metrics
- **If Metrics Server is unavailable:** Automatically falls back to resource requests/limits from pod specifications
- **No errors, no user intervention** - just works with best available data
- Maintains existing ktop behavior where it functions even in clusters without Metrics Server installed

### Prometheus Mode

```bash
ktop --metrics-source=prometheus --enhanced-columns --show-trends
```

Uses Prometheus scraping with enhanced display and trend analysis.

**Important notes:**
- Requires RBAC permissions to access Kubernetes component metrics endpoints
- Requires network access to component endpoints (kubelet, cAdvisor, etc.)
- May not work in managed Kubernetes environments (GKE, EKS, AKS) where component endpoints are restricted
- If scraping fails, shows clear error message (no automatic fallback to metrics-server)
- Opt-in choice for users who need rich metrics and have the necessary permissions

### Prometheus with Custom Configuration

```bash
ktop --metrics-source=prometheus \
     --prometheus-retention=6h \
     --prometheus-components=kubelet,cadvisor,apiserver,etcd \
     --enhanced-columns
```

Extended retention and additional component scraping.

## Implementation Plan

### Phase 1: Foundation

**Adapter Layer**
- Implement `MetricsSource` interface (`k8s/metrics_source.go`)
- Define `NodeMetrics`, `PodMetrics`, `ContainerMetrics` types
- Health monitoring structures

**Prometheus Integration**
- Implement `PromMetricsSource` (`k8s/prom_metrics_source.go`)
- Integrate with `prom.CollectorController`
- Map Prometheus metrics to ktop models
- Health callbacks and error handling

**Metrics Controller**
- Implement `MetricsController` (`k8s/metrics_controller.go`)
- Simple source selection (metrics-server or prometheus)
- Health checking for active source

**Configuration**
- Configuration structures (`config/metrics_config.go`)
- CLI flags (`cmd/ktop.go`)
- Config file loading

### Phase 2: Enhanced UI

**Enhanced Models**
- `EnhancedNodeModel` (`views/model/enhanced_node_model.go`)
- Network, load, disk, container count fields
- Trend calculation
- Health tracking

**Enhanced Panels**
- New columns (`views/overview/enhanced_node_panel.go`)
- Format functions for LOAD, NETWORK, HEALTH, TREND
- Conditional display based on config
- Backward compatible column layout

**Pod and Container Views**
- Container-level metrics
- Throttling and limit info
- Restart counts

### Phase 3: Advanced Features

**Trend Analysis**
- Time-range based calculation
- Resource direction (up/down/flat)
- Configurable windows (5m, 15m, 1h)

**Optimization**
- Efficient storage and retrieval
- Cache management
- Memory overhead minimization
- Performance benchmarks

**Testing**
- Unit tests
- Integration tests with mock components
- Performance benchmarks
- Documentation updates

## Benefits

1. **Backward compatible**: Existing functionality unchanged, enhanced features opt-in.
   - Default behavior (metrics-server) maintains graceful fallback to requests/limits
   - Works in clusters without Metrics Server, just like current ktop

2. **More metrics**: 100+ additional metrics from Kubernetes components (when using prometheus).

3. **Simple and clear**: Explicit source selection, no complex fallback logic.
   - metrics-server: graceful fallback built-in
   - prometheus: explicit choice, explicit errors

4. **Efficient**: In-memory storage with retention limits, minimal overhead.

5. **Flexible**: Configurable scraping, intervals, and display options.

6. **Better visibility**: Health indicators and trend analysis.

7. **Universal compatibility**: Works in any Kubernetes cluster
   - Default mode works even without Metrics Server
   - Prometheus mode for clusters where component endpoints are accessible

## Risks and Mitigations

### Increased Resource Usage

Scraping and storage increase memory footprint.

**Mitigation**: Configurable retention (default 1h), sample limits, selective component scraping, default to metrics-server mode for constrained environments.

### Permission Issues

Scraping requires RBAC permissions that may not exist.

**Mitigation**: Clear error messages when permissions are insufficient, RBAC documentation, user can switch to metrics-server mode via flag.

### Endpoint Discovery

Component endpoints vary across distributions.

**Mitigation**: Leverage prom package discovery, configurable overrides, distribution-specific docs.

### Breaking Changes

New features could break existing workflows.

**Mitigation**: Default behavior unchanged, enhanced features opt-in, regression testing, compatibility documentation.

## Conclusion

Adding Prometheus scraping to ktop expands its metrics capabilities while keeping the tool lightweight and focused. The hybrid approach provides reliability through fallback, compatibility through defaults, and flexibility through configuration. This makes ktop more useful for debugging, capacity planning, and daily cluster operations without changing its core identity as a fast, terminal-based Kubernetes monitor.
