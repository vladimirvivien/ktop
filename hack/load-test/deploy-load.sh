#!/bin/bash
#
# deploy-load.sh - Deploy diverse Kubernetes workloads with real CPU/memory load
#
# Deploys Deployments, StatefulSets, a DaemonSet, Jobs, and a CronJob using
# stress-ng so you can test ktop's Overview, Workloads, Efficiency (EFF), and
# Workload Detail views against live cluster metrics.
#
# Usage:
#   ./deploy-load.sh                          # Deploy diverse workloads with defaults
#   LOAD_PATTERN=spiky ./deploy-load.sh       # Vary CPU intensity
#   LOAD_INCLUDE_FAILED=true ./deploy-load.sh # Also deploy failed/degraded examples
#
set -e

# =============================================================================
# Configuration (override via environment variables)
# =============================================================================

NAMESPACE="${LOAD_NAMESPACE:-load-test}"
WEB_REPLICAS="${LOAD_WEB_REPLICAS:-3}"
API_REPLICAS="${LOAD_API_REPLICAS:-3}"
OVERPROV_REPLICAS="${LOAD_OVERPROV_REPLICAS:-2}"
STS_REPLICAS="${LOAD_STS_REPLICAS:-2}"
BASE_CPU_MILLICORES="${LOAD_CPU_MILLICORES:-100}"
BASE_MEMORY_MB="${LOAD_MEMORY_MB:-64}"
PATTERN="${LOAD_PATTERN:-mixed}"                        # steady, spiky, mixed
STRESS_IMAGE="${LOAD_STRESS_IMAGE:-alexeiled/stress-ng:latest}"
INCLUDE_FAILED="${LOAD_INCLUDE_FAILED:-false}"          # true to also create Failed/Degraded examples

# =============================================================================
# Helper functions
# =============================================================================

print_header() {
    echo ""
    echo "=============================================="
    echo "$1"
    echo "=============================================="
}

print_step() {
    echo "-> $1"
}

print_success() {
    echo "✓ $1"
}

print_info() {
    echo "ℹ $1"
}

get_stress_load() {
    local pattern="$1"
    local default_load="$2"
    case "$pattern" in
        steady) echo "$default_load" ;;
        spiky)  echo "85" ;;
        mixed)  echo "$default_load" ;;
        *)      echo "$default_load" ;;
    esac
}

# =============================================================================
# Main script
# =============================================================================

print_header "ktop Diverse Workloads Deployment"

print_info "Configuration:"
echo "  Namespace:            ${NAMESPACE}"
echo "  Deployments replicas: web=${WEB_REPLICAS}, api=${API_REPLICAS}, overprovisioned=${OVERPROV_REPLICAS}, idle=0"
echo "  StatefulSet replicas: cache-store=${STS_REPLICAS}, queue-broker=${STS_REPLICAS}"
echo "  DaemonSet:            node-load-agent (1 per node)"
echo "  Jobs:                 db-migration (short/complete), batch-cruncher (active)"
echo "  CronJob:              periodic-audit (every 5m)"
echo "  Base CPU / Mem:       ${BASE_CPU_MILLICORES}m / ${BASE_MEMORY_MB}Mi"
echo "  Pattern:              ${PATTERN}"
echo "  Include Failed/Deg:   ${INCLUDE_FAILED}"
echo "  Image:                ${STRESS_IMAGE}"
echo ""

print_step "Creating namespace '${NAMESPACE}'..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
print_success "Namespace ready"

CPU_LIMIT=$((BASE_CPU_MILLICORES * 2))
MEM_LIMIT=$((BASE_MEMORY_MB * 2))
WEB_LOAD=$(get_stress_load "$PATTERN" 70)
API_LOAD=$(get_stress_load "$PATTERN" 90)
STS_MEM=$((BASE_MEMORY_MB > 32 ? BASE_MEMORY_MB : 32))

print_step "Applying Deployments, StatefulSets, DaemonSet, Jobs, and CronJob..."

MANIFEST=$(cat <<EOF
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-frontend
  namespace: ${NAMESPACE}
  labels:
    app: web-frontend
    app.kubernetes.io/part-of: load-test
spec:
  replicas: ${WEB_REPLICAS}
  selector:
    matchLabels:
      app: web-frontend
  template:
    metadata:
      labels:
        app: web-frontend
        workload-type: deployment
    spec:
      containers:
      - name: stress
        image: ${STRESS_IMAGE}
        args: ["--cpu", "1", "--cpu-load", "${WEB_LOAD}", "--vm", "1", "--vm-bytes", "$((BASE_MEMORY_MB * 3 / 4))M", "--vm-hang", "0", "--timeout", "0"]
        resources:
          requests:
            cpu: ${BASE_CPU_MILLICORES}m
            memory: ${BASE_MEMORY_MB}Mi
          limits:
            cpu: ${CPU_LIMIT}m
            memory: ${MEM_LIMIT}Mi
      terminationGracePeriodSeconds: 5
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-server
  namespace: ${NAMESPACE}
  labels:
    app: api-server
    app.kubernetes.io/part-of: load-test
spec:
  replicas: ${API_REPLICAS}
  selector:
    matchLabels:
      app: api-server
  template:
    metadata:
      labels:
        app: api-server
        workload-type: deployment
    spec:
      containers:
      - name: stress
        image: ${STRESS_IMAGE}
        args: ["--cpu", "1", "--cpu-load", "${API_LOAD}", "--vm", "1", "--vm-bytes", "$((BASE_MEMORY_MB - 8))M", "--vm-hang", "0", "--timeout", "0"]
        resources:
          requests:
            cpu: ${BASE_CPU_MILLICORES}m
            memory: ${BASE_MEMORY_MB}Mi
          limits:
            cpu: ${CPU_LIMIT}m
            memory: ${MEM_LIMIT}Mi
      terminationGracePeriodSeconds: 5
---
# Intentionally over-provisioned to exercise low-EFF (<25%) highlighting
apiVersion: apps/v1
kind: Deployment
metadata:
  name: overprovisioned-worker
  namespace: ${NAMESPACE}
  labels:
    app: overprovisioned-worker
    app.kubernetes.io/part-of: load-test
spec:
  replicas: ${OVERPROV_REPLICAS}
  selector:
    matchLabels:
      app: overprovisioned-worker
  template:
    metadata:
      labels:
        app: overprovisioned-worker
        workload-type: deployment
    spec:
      containers:
      - name: stress
        image: ${STRESS_IMAGE}
        args: ["--cpu", "1", "--cpu-load", "5", "--vm", "1", "--vm-bytes", "16M", "--vm-hang", "0", "--timeout", "0"]
        resources:
          requests:
            cpu: 400m
            memory: 256Mi
          limits:
            cpu: 600m
            memory: 512Mi
      terminationGracePeriodSeconds: 5
---
# Scaled-to-zero deployment to exercise Scaled 0 state
apiVersion: apps/v1
kind: Deployment
metadata:
  name: idle-canary
  namespace: ${NAMESPACE}
  labels:
    app: idle-canary
    app.kubernetes.io/part-of: load-test
spec:
  replicas: 0
  selector:
    matchLabels:
      app: idle-canary
  template:
    metadata:
      labels:
        app: idle-canary
        workload-type: deployment
    spec:
      containers:
      - name: stress
        image: ${STRESS_IMAGE}
        args: ["--cpu", "1", "--cpu-load", "10", "--timeout", "0"]
        resources:
          requests:
            cpu: 50m
            memory: 32Mi
---
apiVersion: v1
kind: Service
metadata:
  name: cache-store
  namespace: ${NAMESPACE}
spec:
  clusterIP: None
  selector:
    app: cache-store
  ports:
  - port: 6379
    name: redis
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: cache-store
  namespace: ${NAMESPACE}
  labels:
    app: cache-store
    app.kubernetes.io/part-of: load-test
spec:
  serviceName: cache-store
  replicas: ${STS_REPLICAS}
  selector:
    matchLabels:
      app: cache-store
  template:
    metadata:
      labels:
        app: cache-store
        workload-type: statefulset
    spec:
      containers:
      - name: stress
        image: ${STRESS_IMAGE}
        args: ["--cpu", "1", "--cpu-load", "35", "--vm", "1", "--vm-bytes", "$((STS_MEM / 2))M", "--vm-hang", "0", "--timeout", "0"]
        resources:
          requests:
            cpu: 80m
            memory: ${STS_MEM}Mi
          limits:
            cpu: 200m
            memory: $((STS_MEM * 2))Mi
      terminationGracePeriodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: queue-broker
  namespace: ${NAMESPACE}
spec:
  clusterIP: None
  selector:
    app: queue-broker
  ports:
  - port: 5672
    name: amqp
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: queue-broker
  namespace: ${NAMESPACE}
  labels:
    app: queue-broker
    app.kubernetes.io/part-of: load-test
spec:
  serviceName: queue-broker
  replicas: ${STS_REPLICAS}
  selector:
    matchLabels:
      app: queue-broker
  template:
    metadata:
      labels:
        app: queue-broker
        workload-type: statefulset
    spec:
      containers:
      - name: stress
        image: ${STRESS_IMAGE}
        args: ["--cpu", "1", "--cpu-load", "60", "--vm", "1", "--vm-bytes", "$((STS_MEM * 2 / 3))M", "--vm-hang", "0", "--timeout", "0"]
        resources:
          requests:
            cpu: 100m
            memory: ${STS_MEM}Mi
          limits:
            cpu: 200m
            memory: $((STS_MEM * 2))Mi
      terminationGracePeriodSeconds: 5
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: node-load-agent
  namespace: ${NAMESPACE}
  labels:
    app: node-load-agent
    app.kubernetes.io/part-of: load-test
spec:
  selector:
    matchLabels:
      app: node-load-agent
  template:
    metadata:
      labels:
        app: node-load-agent
        workload-type: daemonset
    spec:
      containers:
      - name: agent
        image: ${STRESS_IMAGE}
        args: ["--cpu", "1", "--cpu-load", "25", "--vm", "1", "--vm-bytes", "24M", "--vm-hang", "0", "--timeout", "0"]
        resources:
          requests:
            cpu: 50m
            memory: 48Mi
          limits:
            cpu: 150m
            memory: 128Mi
      terminationGracePeriodSeconds: 5
---
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migration
  namespace: ${NAMESPACE}
  labels:
    app: db-migration
    app.kubernetes.io/part-of: load-test
spec:
  completions: 1
  template:
    metadata:
      labels:
        app: db-migration
        workload-type: job
    spec:
      restartPolicy: Never
      containers:
      - name: migrate
        image: ${STRESS_IMAGE}
        args: ["--cpu", "1", "--cpu-load", "40", "--timeout", "5s"]
        resources:
          requests:
            cpu: 50m
            memory: 32Mi
          limits:
            cpu: 150m
            memory: 64Mi
---
apiVersion: batch/v1
kind: Job
metadata:
  name: batch-cruncher
  namespace: ${NAMESPACE}
  labels:
    app: batch-cruncher
    app.kubernetes.io/part-of: load-test
spec:
  completions: 2
  parallelism: 1
  template:
    metadata:
      labels:
        app: batch-cruncher
        workload-type: job
    spec:
      restartPolicy: Never
      containers:
      - name: crunch
        image: ${STRESS_IMAGE}
        args: ["--cpu", "1", "--cpu-load", "60", "--vm", "1", "--vm-bytes", "24M", "--vm-hang", "0", "--timeout", "180s"]
        resources:
          requests:
            cpu: 100m
            memory: 48Mi
          limits:
            cpu: 200m
            memory: 128Mi
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: periodic-audit
  namespace: ${NAMESPACE}
  labels:
    app: periodic-audit
    app.kubernetes.io/part-of: load-test
spec:
  schedule: "*/5 * * * *"
  successfulJobsHistoryLimit: 2
  failedJobsHistoryLimit: 1
  jobTemplate:
    spec:
      template:
        metadata:
          labels:
            app: periodic-audit
            workload-type: cronjob
        spec:
          restartPolicy: Never
          containers:
          - name: audit
            image: ${STRESS_IMAGE}
            args: ["--cpu", "1", "--cpu-load", "20", "--timeout", "10s"]
            resources:
              requests:
                cpu: 50m
                memory: 32Mi
EOF
)

if [ "${INCLUDE_FAILED}" = "true" ]; then
    MANIFEST="${MANIFEST}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: stuck-rollout
  namespace: ${NAMESPACE}
  labels:
    app: stuck-rollout
    app.kubernetes.io/part-of: load-test
spec:
  replicas: 2
  selector:
    matchLabels:
      app: stuck-rollout
  template:
    metadata:
      labels:
        app: stuck-rollout
        workload-type: deployment
    spec:
      nodeSelector:
        ktop.io/nonexistent-tier: gold
      containers:
      - name: app
        image: ${STRESS_IMAGE}
        args: [\"--timeout\", \"0\"]
        resources:
          requests:
            cpu: 50m
            memory: 32Mi
---
apiVersion: batch/v1
kind: Job
metadata:
  name: failing-check
  namespace: ${NAMESPACE}
  labels:
    app: failing-check
    app.kubernetes.io/part-of: load-test
spec:
  backoffLimit: 0
  template:
    metadata:
      labels:
        app: failing-check
        workload-type: job
    spec:
      restartPolicy: Never
      containers:
      - name: fail
        image: ${STRESS_IMAGE}
        args: [\"--unknown-flag-for-test\"]
        resources:
          requests:
            cpu: 25m
            memory: 16Mi
"
fi

echo "$MANIFEST" | kubectl apply -f -
print_success "Applied workload manifests to namespace '${NAMESPACE}'"

print_step "Waiting up to 90s for initial workload pods to run..."
TIMEOUT=90
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    RUNNING=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null | grep -c "Running" || true)
    COMPLETED=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null | grep -c "Completed" || true)
    TOTAL=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
    echo "  Running: ${RUNNING}, Completed: ${COMPLETED}, Total Pods: ${TOTAL}"
    if [ "${RUNNING}" -gt 0 ] && [ $((RUNNING + COMPLETED)) -ge "${TOTAL}" ] && [ "${TOTAL}" -gt 0 ]; then
        break
    fi
    sleep 5
    ELAPSED=$((ELAPSED + 5))
done

print_header "Workloads Ready"
kubectl get deployments,statefulsets,daemonsets,jobs,cronjobs -n "${NAMESPACE}"
echo ""
echo "Commands:"
echo "  Watch Workloads: ktop -n ${NAMESPACE} --page=workloads"
echo "  Watch Overview:  ktop -n ${NAMESPACE}"
echo "  Cleanup:         ./hack/load-test/cleanup-load.sh"
