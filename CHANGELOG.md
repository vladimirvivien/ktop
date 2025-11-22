# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Metrics Source Selection**: Configure ktop to use different metrics sources for cluster monitoring
  - `--metrics-source` flag to choose between `metrics-server` (default) or `prometheus`
  - Support for Prometheus-based metrics collection with direct component scraping
  - Backward compatible: defaults to metrics-server with graceful fallback to resource requests/limits

- **Prometheus Configuration Options**: Fine-tune Prometheus metrics collection
  - `--prometheus-scrape-interval` - Configure how often to scrape metrics (default: 15s, minimum: 5s)
  - `--prometheus-retention` - Set how long to keep metrics in memory (default: 1h, minimum: 5m)
  - `--prometheus-max-samples` - Control maximum samples per time series (default: 10000)
  - `--prometheus-components` - Select which Kubernetes components to scrape metrics from
    - Available components: kubelet, cadvisor, apiserver, etcd, scheduler, controller-manager, kube-proxy
    - Default: kubelet, cadvisor

- **Configuration System**: New `config` package for managing metrics source configuration
  - Centralized configuration with validation
  - Support for default values with CLI flag overrides
  - Component type parsing and validation

- **Column Filtering**: Customize which columns are displayed in the nodes and pods tables
  - `--node-columns` - Show only specific node columns (e.g., `NAME,CPU,MEM`)
  - `--pod-columns` - Show only specific pod columns (e.g., `NAMESPACE,POD,STATUS`)
  - `--show-all-columns` - Toggle showing all columns (default: true)

### Changed

- **Application Architecture**: Application now uses MetricsSource interface for all metrics operations
  - Views properly connected to MetricsSource for fresh metrics
  - Graceful degradation when metrics sources are unavailable
  - Improved separation of concerns between metrics collection and display
- Enhanced CLI help text with detailed Prometheus configuration options
- Improved error messages for invalid configuration values

### Fixed

- **Critical**: Fixed application startup hang (2-10 second delay)
  - Removed metrics informers from controller (now handled by MetricsSource interface)
  - Eliminated blocking `AssertMetricsAvailable()` network call during controller startup
  - Removed 5-second timeout waiting for metrics informers to sync
  - Core resource informers use 2-second timeout instead of indefinite wait
  - Removed blocking `AssertMetricsAvailable()` call from application setup
  - Application now checks MetricsSource health asynchronously
  - Startup is now near-instant (<2 seconds) regardless of cluster size or metrics availability
  - Added "Loading cluster data..." message to inform users during initial data load
- **Critical**: Fixed metrics connection status display
  - Status now reflects actual MetricsSource health (not cached k8s client state)
  - Shows "not connected" when Metrics Server is truly unavailable
  - MetricsServerSource starts unhealthy, becomes healthy after first successful fetch
  - Header auto-updates after 2 seconds to show "connected" once metrics are fetched
  - Periodic refresh every 10 seconds to keep status current
- **Critical**: Fixed cluster summary showing 0m/0Gi for CPU/Memory usage
  - Controller now uses MetricsSource interface for cluster summary metrics
  - `Controller.GetNodeMetrics()` updated to fetch from MetricsSource instead of deprecated informers
  - Fixed nil pointer dereference panic when metrics are unavailable
  - Gracefully skips nodes without metrics instead of crashing
  - Works correctly with all metrics sources (metrics-server, prometheus, or fallback)
  - Cluster summary now displays actual usage data from configured metrics source
- **Critical**: Fixed infinite recursion stack overflow in MetricsServerSource
  - `MetricsServerSource` now calls Kubernetes Metrics Server API directly
  - Removed circular dependency between Controller and MetricsServerSource
  - Fixed stack overflow: `Controller.GetNodeMetrics()` → `MetricsSource.GetNodeMetrics()` → `Controller.GetNodeMetrics()` loop
  - `MetricsServerSource` now accesses `metricsClient` directly instead of through controller
  - All metrics fetching methods updated (GetNodeMetrics, GetPodMetrics, GetAllPodMetrics)
- Fixed cluster summary display in fallback mode
  - Now shows resource requests when usage metrics are unavailable (0m/0Gi)
  - Changed from blocking `AssertMetricsAvailable()` check to data-based check
  - Checks if usage values are non-zero instead of API availability
  - Properly displays "% requested" vs "% used" based on actual data availability
- **Critical**: Fixed memory display showing incorrect values across all views
  - Changed from decimal Giga (10^9) to binary GiB (2^30) for memory calculations
  - Added smart unit formatting: displays Mi for values <10Gi, Gi for larger values
  - This matches `kubectl top` behavior and prevents loss of precision
  - Fixed cluster summary memory display (e.g., 1405Mi/15Gi instead of 2Gi/17Gi)
  - Fixed nodes panel memory display (both usage and allocatable)
  - Fixed pods panel memory display showing actual values (366Mi, 56Mi, etc. instead of 0Gi)
  - Fixed nodes panel incorrectly using cached `AssertMetricsAvailable()` check
  - Nodes panel now checks actual data presence instead of relying on stale API checks
  - Memory values now match `kubectl top nodes` and `kubectl top pods` output exactly
  - Fixes both "% requested" and "% used" memory display modes across all panels
  - Added `ui.FormatMemory()` helper function with comprehensive tests
- Fixed issue where CPU and Memory columns showed "Unavailable" in default mode
  - Properly connected metrics sources to UI views
  - MetricsSource now correctly passed from CLI → Application → Views
- Metrics Server graceful fallback to requests/limits now works correctly
  - When Metrics Server unavailable, automatically uses resource requests/limits
  - No error messages shown to user, seamless experience
- Fixed pod panel display formatting to match previous version exactly
  - VOLS column format: `2/2` (volumes/mounts)
  - CPU/Memory format includes bargraph and percentage
  - Proper unit handling (Giga for memory)

### Notes

- **Prometheus Enhanced Metrics**: While Prometheus metrics collection is fully functional, enhanced UI columns (network I/O, load averages, container counts) are planned for a future release. Use `--enhanced-columns` flag (when available) to access these additional metrics.
- **Managed Kubernetes**: Prometheus mode may not work in managed Kubernetes environments (GKE, EKS, AKS) where component endpoints are restricted. Use default metrics-server mode in these environments.
- **RBAC Requirements**: Prometheus mode requires RBAC permissions to access component `/metrics` endpoints. If permissions are insufficient, use metrics-server mode.

## [0.3.0] - Previous Release

See git history for details of previous releases.
