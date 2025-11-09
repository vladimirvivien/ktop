# ktop Roadmap

This document outlines the development plans and enhancements for ktop, a top-like tool for Kubernetes clusters.

## Vision

ktop is a terminal-based monitoring tool for Kubernetes clusters, following the Unix/Linux `top` tradition. It provides real-time, deep metrics views of cluster resources with minimal overhead and maximum utility.

---

## Current Status (Completed)

### Foundation Layer ✓
- [x] Core Prometheus metrics collection package (`prom/`)
  - [x] Scraper implementation for Kubernetes components
  - [x] In-memory metrics storage with retention management
  - [x] Controller for managing collection lifecycle
  - [x] Support for kubelet, cAdvisor, API server, etcd, scheduler, controller-manager, kube-proxy
- [x] Metrics source interface abstraction (`metrics/source.go`)
- [x] MetricsServer source implementation (`metrics/k8s/metrics_server_source.go`)
- [x] Prometheus metrics source implementation (`metrics/prom/prom_source.go`)
- [x] Comprehensive test coverage (>80%) for adapter layer

---

## Phase 1: Complete Prometheus Integration

**Goal:** Add Prometheus metrics collection as an alternative to metrics-server

> See [docs/prom.md](docs/prom.md) for detailed technical design and implementation specifications.
> See [.claude/prom-impl-plan.md](.claude/prom-impl-plan.md) for week-by-week implementation details.

**Status:** ~30% complete (Week 1 done, Week 2-3 in progress)

### Adapter Layer (75% complete)
- [x] Create MetricsSource interface (`metrics/source.go`)
- [x] Implement PromMetricsSource (`metrics/prom/prom_source.go`)
- [x] Implement MetricsServerSource (`metrics/k8s/metrics_server_source.go`)
- [ ] Integrate source selection into application startup

### CLI Integration (0% complete)
- [ ] Add command-line flags for metrics source selection
  - [ ] `--metrics-source` (metrics-server | prometheus)
  - [ ] `--prometheus-scrape-interval`
  - [ ] `--prometheus-retention`
  - [ ] `--prometheus-max-samples`
  - [ ] `--prometheus-components`
- [ ] Create configuration system (`config/config.go`)
- [ ] Configuration file support (YAML) - deferred to later phase

### Enhanced Metrics Display (0% complete)
- [ ] Update node view with Prometheus metrics
  - [ ] Network I/O (Rx/Tx bytes)
  - [ ] Load averages (1m, 5m, 15m)
  - [ ] Container counts
  - [ ] Disk usage
- [ ] Update pod view with enhanced metrics
  - [ ] Per-container CPU/memory breakdown
  - [ ] CPU throttling indicators
  - [ ] Container restart counts
- [ ] Add optional enhanced columns (enabled via `--enhanced-columns`)
- [ ] Show active metrics source indicator

### Testing & Documentation (25% complete)
- [x] Unit tests for adapter layer (MetricsServerSource, PromMetricsSource)
- [x] CI/CD pipeline with automated testing
- [ ] Integration tests with real clusters
- [ ] Update README with Prometheus features
- [ ] Document available metrics and configuration options
- [ ] RBAC permissions documentation

---

## Phase 2: Deep Metrics Views

**Goal:** Provide detailed, actionable metrics following the `top` philosophy

### Historical Trends
- [ ] Show resource usage trends (up/down/stable indicators)
- [ ] Calculate trends over configurable windows (5m, 15m, 1h)
- [ ] Simple ASCII sparklines for CPU/memory trends
- [ ] Minimal historical data retention for trend analysis

### Advanced Filtering & Sorting
- [ ] Filter by resource thresholds (high CPU/memory usage)
- [ ] Filter by network I/O rates
- [ ] Multi-column sorting
- [ ] Save filter preferences

---

## Phase 3: Additional Views

**Goal:** Extend top-like views to more Kubernetes resources

### Workload Views
- [ ] Deployment view (replica status, resource usage per deployment)
- [ ] StatefulSet view
- [ ] DaemonSet view
- [ ] Job/CronJob view

### Namespace View
- [ ] Namespace-level resource aggregation
- [ ] Quota utilization tracking
- [ ] Per-namespace metrics breakdown

### Storage View
- [ ] PV/PVC usage statistics
- [ ] Storage capacity and utilization

### Navigation
- [ ] Tab-based navigation between views
- [ ] Keyboard shortcuts for switching views
- [ ] Quick search/filter across resources

---

## Phase 4: Enhanced Monitoring

**Goal:** Add deeper monitoring capabilities while staying lightweight

### OOM Detection
- [ ] Detect and highlight OOM-killed processes
- [ ] Show OOM event history
- [ ] Display memory pressure indicators

### Performance Insights
- [ ] Show container CPU throttling
- [ ] Display I/O wait times
- [ ] Network errors and retransmits
- [ ] API server latency metrics
- [ ] etcd performance metrics

### Detail Panels
- [ ] Node detail view (pods on node, allocatable resources)
- [ ] Pod detail view (container breakdown, events)
- [ ] Resource utilization vs requests/limits comparison

---

## Distribution

### Current ✓
- [x] kubectl plugin via krew
- [x] Homebrew formula (macOS/Linux)
- [x] Container image
- [x] Go install support

### Planned
- [ ] Linux distribution packages
  - [ ] .deb packages (Debian/Ubuntu)
  - [ ] .rpm packages (RHEL/Fedora/CentOS)
  - [ ] Arch Linux AUR
- [ ] Windows packages
  - [ ] Chocolatey
  - [ ] Scoop

---

## Quality & Performance

### Testing
- [ ] Expand unit test coverage (target >80%)
- [ ] Integration tests for all metrics sources
- [ ] Performance tests (large clusters, low resource usage)

### Performance Goals
- [ ] Startup time: <2 seconds
- [ ] Memory usage: <100MB for typical cluster (100 nodes)
- [ ] CPU overhead: <5% of single core
- [ ] UI responsiveness: <100ms updates

### Code Quality
- [ ] Linting and formatting standards
- [ ] Security scanning
- [ ] Dependency vulnerability checks

---

## Documentation

- [ ] User guide (getting started, configuration, troubleshooting)
- [ ] Architecture documentation
- [ ] Available metrics reference
- [ ] Performance tuning guide
- [ ] Contributing guidelines

---

## Non-Goals

To keep ktop focused, the following are **not** planned:
- Integration with external monitoring systems (Grafana, Datadog, etc.)
- Alerting or notification systems
- Log aggregation or viewing
- Cluster management or modification capabilities
- Plugin systems or extensibility frameworks
- Multi-cluster dashboards
- Export/reporting features

---

## Contributing

This roadmap focuses on making ktop the best terminal-based Kubernetes metrics viewer. We welcome contributions aligned with this vision.

To suggest changes, please open a GitHub issue or pull request.

---

## Implementation Notes

### Phase 1 Approach

**Simplified Two-Source Model:**
- Users explicitly select **one** metrics source at startup via `--metrics-source` flag
- Default: `metrics-server` (backward compatible, no changes to existing behavior)
- Opt-in: `prometheus` (enhanced metrics from Kubernetes components)
- **No hybrid mode** - simple explicit choice, easier to debug
- **No auto-fallback** - if selected source fails, show clear error message

**Week-by-Week Progress:**
- ✅ Week 1 (Complete): Adapter layer foundation
  - MetricsSource interface
  - MetricsServerSource implementation
  - PromMetricsSource implementation
  - Comprehensive tests (>80% coverage)
- 🔄 Week 2 (In Progress): Configuration and CLI integration
  - Configuration system
  - CLI flags
  - Source initialization
  - Documentation updates
- ⏳ Week 3 (Planned): Enhanced UI views
  - Enhanced node/pod models
  - Optional enhanced columns
  - Source health indicators
  - Integration testing

---

**Last Updated:** November 9, 2025
**Status:** Active Development - Phase 1 in progress (~30% complete)
