# Load Test for ktop

Deploy diverse Kubernetes workloads (`Deployment`, `StatefulSet`, `DaemonSet`, `Job`, and `CronJob`) along with live Storage (`PersistentVolumeClaim`, `PersistentVolume`, disk I/O pods) and Network (`Service`, `EndpointSlice`, `Ingress`, HTTP traffic generator) fixtures for testing `ktop`'s `Overview`, `Workloads`, `Storage`, `Network`, and `Pod Detail` views.

## Quick Start

```bash
# Deploy workloads, storage, and network fixtures into the load-test namespace
./hack/load-test/deploy-load.sh

# Launch directly onto individual tabs
ktop -n load-test --page=workloads
ktop -n load-test --page=storage
ktop -n load-test --page=network

# Inspect headless --noui JSON summary
ktop --noui | jq '{storage: .storage.summary, network: .network.summary}'

# Cleanup when done (removes namespace and labeled PersistentVolumes)
./hack/load-test/cleanup-load.sh
```

## Deployed Resources

### Compute Workloads

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

### Storage Fixtures (`LOAD_INCLUDE_STORAGE=true`)

| Kind | Name | Purpose |
|------|------|---------|
| `PersistentVolumeClaim` | `app-data-pvc` (`256Mi`) | Dynamically provisioned PVC with ~32Mi seeded data and active disk I/O |
| `PersistentVolumeClaim` | `archive-logs-pvc` (`128Mi`) | Dynamically provisioned PVC with 500 files for `INODES%` and byte telemetry |
| `PersistentVolume` | `<namespace>-available-pv` (`1Gi`) | Cluster-scoped static `hostPath` PV (`Retain`, `Available`) for the PVs table |
| `Deployment` | `storage-writer` | Mounts `app-data-pvc`, writes/reads `2Mi` blocks every `2s` for Disk R/W and Pod Detail `PVCs:` |
| `Deployment` | `storage-archiver` | Mounts `archive-logs-pvc`, seeds 500 log files, and writes periodic appends |

### Network Fixtures (`LOAD_INCLUDE_NETWORK=true`)

| Kind | Name | Type / Role | Purpose |
|------|------|-------------|---------|
| `Deployment` | `net-echo-server` (`2` replicas) | `httpd` on `:8080` | Serves a `256KiB` `/payload.bin` endpoint with TCP readiness probe |
| `Deployment` | `net-traffic-gen` (`2` replicas) | HTTP client loop | Polls `http://net-echo-svc/payload.bin` every `1s` to drive live cAdvisor RX/TX traffic |
| `Service` | `net-echo-svc` | `ClusterIP` (`2/2` ready) | Healthy service backing HTTP traffic |
| `Service` | `web-frontend-svc` | `NodePort` (`3/3` ready) | Exercises `NodePort` port mapping (`80:<nodePort>/TCP`) |
| `Service` | `api-public-svc` | `LoadBalancer` (`2/2` ready) | Exercises `EXTERNAL-IP` rendering when `minikube tunnel` or MetalLB is running |
| `Service` | `external-db-svc` | `ExternalName` | Verifies external hostname services do not trigger zero-endpoint alerts |
| `Service` | `orphan-backend-svc` | `ClusterIP` (`0/0` ready) | Selector matches zero pods to trigger zero-endpoint alert highlighting |
| `Ingress` | `load-test-web-ingress` | `nginx` (`80, 443`) | TLS-configured Ingress routing `web.load-test.local` |
| `Ingress` | `load-test-api-ingress` | `nginx` (`80`) | Multi-host HTTP Ingress routing `api.load-test.local` and `status.load-test.local` |

Set `LOAD_INCLUDE_FAILED=true` to also deploy `stuck-rollout` (unschedulable `Deployment`), `failing-check` (failing `Job`), and `pending-fast-pvc` (`Pending` PVC referencing a non-existent StorageClass) for testing degraded/failed/pending indicators:

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
| `LOAD_INCLUDE_STORAGE` | `true` | Set `false` to skip Storage PVCs, PV, and disk I/O pods |
| `LOAD_INCLUDE_NETWORK` | `true` | Set `false` to skip Services, Ingresses, and HTTP traffic generators |
| `LOAD_STORAGE_CLASS` | *(empty)* | Explicit `storageClassName` for test PVCs (uses cluster default when empty) |
| `LOAD_INCLUDE_FAILED` | `false` | Set `true` to include unschedulable, failing, and pending test fixtures |
| `LOAD_STRESS_IMAGE` | `alexeiled/stress-ng:latest` | Stress container image |
| `LOAD_UTILITY_IMAGE` | `busybox:1.36` | Utility image for disk I/O, HTTP echo server, and HTTP traffic generator |

## Local Cluster Tips (`minikube` vs `kind`)

Both `kind` and `minikube` work with this script, though `minikube` exposes richer network visibility locally:

- **Ingress addresses (`minikube`)**: Run `minikube addons enable ingress` so the Ingress controller reconciles `load-test-web-ingress` and `load-test-api-ingress` with a populated `ADDRESS`.
- **LoadBalancer external IPs (`minikube`)**: Run `minikube tunnel` in a separate terminal to assign an `EXTERNAL-IP` to `api-public-svc`.
- **Per-node network counters**: Running `minikube` with a VM driver (`vfkit`, `qemu`, or `kvm2`) gives each node an isolated virtual NIC for cAdvisor `container_network_*` metrics, whereas container-based clusters share the Docker host VM network stack.
- **Volume capacity on `hostPath` provisioners**: Both `kind` (`local-path-provisioner`) and default `minikube` (`minikube-hostpath`) report the node root filesystem capacity rather than the PVC request size in `kubelet_volume_stats_capacity_bytes`, so `USED` and inode counts reflect actual writes while `FILL%` stays proportional to host disk capacity unless backed by a size-bounded block filesystem.

## Cleanup

```bash
./hack/load-test/cleanup-load.sh
```
