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

- Enhanced CLI help text with detailed Prometheus configuration options
- Improved error messages for invalid configuration values

### Notes

- **Prometheus UI Integration**: Prometheus metrics source is initialized and collects data, but UI integration is not yet complete. The application will show a warning and use metrics-server fallback for display while this feature is being completed.
- **Managed Kubernetes**: Prometheus mode may not work in managed Kubernetes environments (GKE, EKS, AKS) where component endpoints are restricted. Use default metrics-server mode in these environments.
- **RBAC Requirements**: Prometheus mode requires RBAC permissions to access component `/metrics` endpoints. If permissions are insufficient, use metrics-server mode.

## [0.3.0] - Previous Release

See git history for details of previous releases.
