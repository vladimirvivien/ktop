# Load Test for ktop

Deploy diverse Kubernetes workloads (`Deployment`, `StatefulSet`, `DaemonSet`, `Job`, and `CronJob`) generating live CPU and memory consumption using [stress-ng](https://github.com/ColinIanKing/stress-ng) for testing both the `Overview` and `Workloads` tabs in `ktop`.

## Quick Start

```bash
# Deploy diverse workloads into the load-test namespace
./hack/load-test/deploy-load.sh

# Launch directly onto the Workloads tab
ktop -n load-test --page=workloads

# Or watch cluster Overview + switch tabs with '1' / '2'
ktop -n load-test

# Cleanup when done
./hack/load-test/cleanup-load.sh
```

## Deployed Workloads

| Kind | Name | Default Scale | Purpose |
|------|------|---------------|---------|
| `Deployment` | `web-frontend` | `3` replicas | Balanced CPU/memory utilization (~70% load) |
| `Deployment` | `api-server` | `3` replicas | High CPU/memory pressure (~90% load) |
| `Deployment` | `overprovisioned-worker` | `2` replicas | Large requests (`400m` / `256Mi`) vs minimal usage (`EFF < 25%`) |
| `Deployment` | `idle-canary` | `0` replicas | Zero-replica (`Scaled 0`) state |
| `StatefulSet` | `cache-store` | `2` replicas | Steady memory-backed StatefulSet with headless Service |
| `StatefulSet` | `queue-broker` | `2` replicas | Mixed CPU/memory StatefulSet with headless Service |
| `DaemonSet` | `node-load-agent` | `1` per node | Node-wide DaemonSet coverage |
| `Job` | `db-migration` | `1/1` completion | Short job that transitions to `Complete` |
| `Job` | `batch-cruncher` | `2` completions (sequential) | Multi-minute `Running` job |
| `CronJob` | `periodic-audit` | Every `5m` | Generates recurring scheduled Jobs |

Set `LOAD_INCLUDE_FAILED=true` to also deploy `stuck-rollout` (unschedulable `Deployment`) and `failing-check` (failing `Job`) for testing degraded/failed status indicators:

```bash
LOAD_INCLUDE_FAILED=true ./hack/load-test/deploy-load.sh
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LOAD_NAMESPACE` | `load-test` | Kubernetes namespace for test resources |
| `LOAD_WEB_REPLICAS` | `3` | Replicas for `web-frontend` Deployment |
| `LOAD_API_REPLICAS` | `3` | Replicas for `api-server` Deployment |
| `LOAD_OVERPROV_REPLICAS` | `2` | Replicas for `overprovisioned-worker` Deployment |
| `LOAD_STS_REPLICAS` | `2` | Replicas for each StatefulSet |
| `LOAD_CPU_MILLICORES` | `100` | Base CPU request per standard pod |
| `LOAD_MEMORY_MB` | `64` | Base memory request per standard pod |
| `LOAD_PATTERN` | `mixed` | Load intensity preset: `steady`, `spiky`, or `mixed` |
| `LOAD_INCLUDE_FAILED` | `false` | Set `true` to include unschedulable/failing test workloads |
| `LOAD_STRESS_IMAGE` | `alexeiled/stress-ng:latest` | Stress container image |

## Cleanup

```bash
./hack/load-test/cleanup-load.sh
```
