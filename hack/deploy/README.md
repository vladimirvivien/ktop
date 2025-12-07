# ktop Test Users and RBAC

This directory contains RBAC configurations for testing ktop with different permission levels.

## Files

| File | Description |
|------|-------------|
| `ktop-user.yaml` | Two test service accounts with different access levels |
| `generate-minikube-kubeconfig.sh` | Helper script to generate kubeconfig for minikube |
| `rbac-prometheus.yaml` | ClusterRole for Prometheus scraping (reference) |

## Test Users

The `ktop-user.yaml` creates two service accounts for testing metrics source fallback:

| User | Prometheus | Metrics-Server | Use Case |
|------|------------|----------------|----------|
| `ktop-full` | Yes | Yes | Full access, tests default prometheus path |
| `ktop-restricted` | No | Yes | Tests fallback from prometheus to metrics-server |

## Setup

### 1. Apply the RBAC configuration

```bash
kubectl apply -f ktop-user.yaml
```

### 2. Generate kubeconfig for test users (minikube)

**For ktop-full (full access):**

```bash
./generate-minikube-kubeconfig.sh ktop-full
```

**For ktop-restricted (metrics-server only):**

```bash
./generate-minikube-kubeconfig.sh ktop-restricted
```

Or manually:

```bash
# Set the user (ktop-full or ktop-restricted)
USER=ktop-restricted

# Generate token (valid for 24 hours)
TOKEN=$(kubectl create token $USER -n ktop-test --duration=24h)

# Get cluster info
CLUSTER_SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
CLUSTER_CA=$(kubectl config view --minify --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')

# Create kubeconfig
cat > /tmp/ktop-${USER}.kubeconfig <<EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: ${CLUSTER_CA}
    server: ${CLUSTER_SERVER}
  name: ktop-test
contexts:
- context:
    cluster: ktop-test
    user: ${USER}
    namespace: default
  name: ${USER}
current-context: ${USER}
users:
- name: ${USER}
  user:
    token: ${TOKEN}
EOF

echo "Created /tmp/ktop-${USER}.kubeconfig"
```

## Testing Metrics Source Fallback

### Test 1: Full access user (prometheus works)

```bash
# Should use prometheus successfully
ktop --kubeconfig=/tmp/ktop-ktop-full.kubeconfig

# Expected output:
# Connected to: https://...
# Connecting to Prometheus... ✓
```

### Test 2: Restricted user with default (fallback to metrics-server)

```bash
# Should fail prometheus, fallback to metrics-server
ktop --kubeconfig=/tmp/ktop-ktop-restricted.kubeconfig

# Expected output:
# Connected to: https://...
# Connecting to Prometheus... ✗
# Falling back to Metrics Server... ✓
```

### Test 3: Restricted user with explicit prometheus (should fail)

```bash
# Should fail with error (no fallback when explicit)
ktop --kubeconfig=/tmp/ktop-ktop-restricted.kubeconfig --metrics-source=prometheus

# Expected output:
# Connected to: https://...
# Connecting to Prometheus... ✗
# Error: prometheus not available: ...
```

### Test 4: Restricted user with explicit metrics-server (should work)

```bash
# Should work directly
ktop --kubeconfig=/tmp/ktop-ktop-restricted.kubeconfig --metrics-source=metrics-server

# Expected output:
# Connected to: https://...
# Connecting to Metrics Server... ✓
```

### Test 5: No metrics mode

```bash
# Should work, shows resource requests/limits only
ktop --kubeconfig=/tmp/ktop-ktop-restricted.kubeconfig --metrics-source=none

# Expected output:
# Connected to: https://...
# Using metrics source: None
```

## Cleanup

```bash
kubectl delete -f ktop-user.yaml
rm /tmp/ktop-ktop-*.kubeconfig
```

## RBAC Permissions Reference

### Prometheus scraping requires

| Resource | API Group | Verbs | Purpose |
|----------|-----------|-------|---------|
| `nodes` | `""` | get, list | Node discovery |
| `nodes/proxy` | `""` | get | Kubelet/cAdvisor metrics via `/api/v1/nodes/{node}/proxy/metrics` |
| `pods` | `""` | get, list | Pod discovery |
| `pods/proxy` | `""` | get | Component metrics via `/api/v1/namespaces/{ns}/pods/{pod}/proxy/metrics` |
| `/metrics` | (non-resource) | get | API server metrics |

### Metrics-server requires

| Resource | API Group | Verbs | Purpose |
|----------|-----------|-------|---------|
| `nodes` | `metrics.k8s.io` | get, list | Node metrics |
| `pods` | `metrics.k8s.io` | get, list | Pod metrics |
