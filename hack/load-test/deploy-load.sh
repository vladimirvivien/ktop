#!/bin/bash
#
# deploy-load.sh - Deploy real pods with CPU/memory workloads for ktop testing
#
# This script creates pods that generate actual resource consumption using stress-ng,
# providing realistic metrics for testing ktop's visualization capabilities.
#
# Usage:
#   ./deploy-load.sh                          # Deploy with defaults
#   LOAD_POD_COUNT=50 ./deploy-load.sh        # Deploy 50 pods
#   LOAD_PATTERN=spiky ./deploy-load.sh       # Deploy with spiky workload pattern
#
set -e

# =============================================================================
# Configuration (override via environment variables)
# =============================================================================

NAMESPACE="${LOAD_NAMESPACE:-load-test}"
POD_COUNT="${LOAD_POD_COUNT:-20}"
BASE_CPU_MILLICORES="${LOAD_CPU_MILLICORES:-100}"      # Base CPU request per pod
BASE_MEMORY_MB="${LOAD_MEMORY_MB:-64}"                  # Base memory per pod
PATTERN="${LOAD_PATTERN:-steady}"                       # steady, spiky, ramping, mixed
DURATION="${LOAD_DURATION:-0}"                          # 0 = run forever

# Stress-ng image (lightweight, widely available)
STRESS_IMAGE="${LOAD_STRESS_IMAGE:-alexeiled/stress-ng:latest}"

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
    echo "→ $1"
}

print_success() {
    echo "✓ $1"
}

print_info() {
    echo "ℹ $1"
}

# Generate stress-ng arguments based on pattern
get_stress_args() {
    local pattern="$1"
    local cpu_workers="$2"
    local mem_mb="$3"
    local pod_index="$4"

    case "$pattern" in
        steady)
            # Constant load - uses specified CPU and memory continuously
            echo "--cpu ${cpu_workers} --vm 1 --vm-bytes ${mem_mb}M --vm-hang 0 --timeout 0"
            ;;
        spiky)
            # Spiky load - cycles between low and high load
            # Uses --cpu-load to vary CPU percentage (30-90%)
            local load=$((30 + (pod_index * 7) % 60))
            echo "--cpu ${cpu_workers} --cpu-load ${load} --vm 1 --vm-bytes ${mem_mb}M --vm-hang 0 --timeout 0"
            ;;
        ramping)
            # Ramping - different pods start at different load levels
            # Simulates gradual cluster load increase
            local load=$((20 + (pod_index * 4) % 80))
            echo "--cpu ${cpu_workers} --cpu-load ${load} --vm 1 --vm-bytes ${mem_mb}M --vm-hang 0 --timeout 0"
            ;;
        mixed)
            # Mixed patterns - each pod gets a random pattern
            local patterns=("steady" "spiky" "ramping")
            local selected_pattern="${patterns[$((pod_index % 3))]}"
            get_stress_args "$selected_pattern" "$cpu_workers" "$mem_mb" "$pod_index"
            ;;
        *)
            echo "--cpu ${cpu_workers} --vm 1 --vm-bytes ${mem_mb}M --vm-hang 0 --timeout 0"
            ;;
    esac
}

# =============================================================================
# Main script
# =============================================================================

print_header "ktop Load Test Deployment"

print_info "Configuration:"
echo "  Namespace:     ${NAMESPACE}"
echo "  Pod count:     ${POD_COUNT}"
echo "  CPU request:   ${BASE_CPU_MILLICORES}m per pod"
echo "  Memory:        ${BASE_MEMORY_MB}Mi per pod"
echo "  Pattern:       ${PATTERN}"
echo "  Image:         ${STRESS_IMAGE}"
echo ""

# Step 1: Create namespace
print_step "Creating namespace '${NAMESPACE}'..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
print_success "Namespace ready"

# Step 2: Generate and apply pod manifests
print_step "Deploying ${POD_COUNT} load-test pods..."

# Calculate resource limits (2x requests for headroom)
CPU_LIMIT=$((BASE_CPU_MILLICORES * 2))
MEMORY_LIMIT=$((BASE_MEMORY_MB * 2))

# Generate all pods in a single manifest
MANIFEST=""

for i in $(seq 0 $((POD_COUNT - 1))); do
    POD_NAME=$(printf "load-%04d" $i)

    # Get stress arguments for this pod's pattern
    STRESS_ARGS=$(get_stress_args "$PATTERN" 1 "$BASE_MEMORY_MB" "$i")

    # Vary resources slightly for more realistic distribution (±20%)
    VARIATION=$((i % 5))
    POD_CPU=$((BASE_CPU_MILLICORES + (VARIATION - 2) * 20))
    POD_MEM=$((BASE_MEMORY_MB + (VARIATION - 2) * 10))
    POD_CPU_LIMIT=$((POD_CPU * 2))
    POD_MEM_LIMIT=$((POD_MEM * 2))

    # Ensure minimums
    [ $POD_CPU -lt 50 ] && POD_CPU=50
    [ $POD_MEM -lt 32 ] && POD_MEM=32

    MANIFEST="${MANIFEST}
---
apiVersion: v1
kind: Pod
metadata:
  name: ${POD_NAME}
  namespace: ${NAMESPACE}
  labels:
    app: load-test
    pattern: ${PATTERN}
    index: \"${i}\"
spec:
  containers:
  - name: stress
    image: ${STRESS_IMAGE}
    args: [$(echo "$STRESS_ARGS" | sed 's/ /", "/g' | sed 's/^/"/' | sed 's/$/"/' )]
    resources:
      requests:
        cpu: ${POD_CPU}m
        memory: ${POD_MEM}Mi
      limits:
        cpu: ${POD_CPU_LIMIT}m
        memory: ${POD_MEM_LIMIT}Mi
  restartPolicy: Always
  terminationGracePeriodSeconds: 5
"
done

# Apply all pods at once
echo "$MANIFEST" | kubectl apply -f -

print_success "Submitted ${POD_COUNT} pods"

# Step 3: Wait for pods to start
print_step "Waiting for pods to start..."

# Wait up to 2 minutes for pods to be running
TIMEOUT=120
INTERVAL=5
ELAPSED=0

while [ $ELAPSED -lt $TIMEOUT ]; do
    RUNNING=$(kubectl get pods -n "${NAMESPACE}" -l app=load-test --no-headers 2>/dev/null | grep -c "Running" || echo "0")
    PENDING=$(kubectl get pods -n "${NAMESPACE}" -l app=load-test --no-headers 2>/dev/null | grep -c "Pending" || echo "0")
    TOTAL=$(kubectl get pods -n "${NAMESPACE}" -l app=load-test --no-headers 2>/dev/null | wc -l | tr -d ' ')

    echo "  Running: ${RUNNING}/${TOTAL} (Pending: ${PENDING})"

    if [ "$RUNNING" -eq "$POD_COUNT" ]; then
        break
    fi

    sleep $INTERVAL
    ELAPSED=$((ELAPSED + INTERVAL))
done

if [ "$RUNNING" -eq "$POD_COUNT" ]; then
    print_success "All ${POD_COUNT} pods are running"
else
    echo "⚠ Warning: Only ${RUNNING}/${POD_COUNT} pods running after ${TIMEOUT}s timeout"
    echo "  Check pod status: kubectl get pods -n ${NAMESPACE}"
fi

# Step 4: Show summary
print_header "Deployment Complete"

echo ""
echo "Total resources requested:"
TOTAL_CPU=$((BASE_CPU_MILLICORES * POD_COUNT))
TOTAL_MEM=$((BASE_MEMORY_MB * POD_COUNT))
echo "  CPU:    ${TOTAL_CPU}m (~$((TOTAL_CPU / 1000)) cores)"
echo "  Memory: ${TOTAL_MEM}Mi (~$((TOTAL_MEM / 1024))Gi)"
echo ""
echo "Commands:"
echo "  View pods:     kubectl get pods -n ${NAMESPACE}"
echo "  Pod metrics:   kubectl top pods -n ${NAMESPACE}"
echo "  Watch ktop:    ktop -n ${NAMESPACE}"
echo "  Cleanup:       ./hack/load-test/cleanup-load.sh"
echo ""
echo "Tip: Run 'ktop' to see the load in action!"
