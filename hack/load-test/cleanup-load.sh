#!/bin/bash
#
# cleanup-load.sh - Remove all load-test workloads, pods, and namespace
#
# Usage:
#   ./cleanup-load.sh                         # Cleanup default namespace
#   LOAD_NAMESPACE=custom ./cleanup-load.sh   # Cleanup custom namespace
#
set -e

NAMESPACE="${LOAD_NAMESPACE:-load-test}"
PV_SELECTOR="app.kubernetes.io/part-of=load-test,load-test-namespace=${NAMESPACE}"

echo "=== Cleaning up load-test workloads ==="
echo ""

PV_COUNT=$(kubectl get pv -l "${PV_SELECTOR}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [ "${PV_COUNT}" -gt 0 ]; then
    echo "-> Deleting ${PV_COUNT} labeled PersistentVolume(s)..."
    kubectl delete pv -l "${PV_SELECTOR}" --ignore-not-found --wait=false
fi

if ! kubectl get namespace "${NAMESPACE}" &>/dev/null; then
    echo "ℹ Namespace '${NAMESPACE}' does not exist."
    exit 0
fi

POD_COUNT=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
WL_COUNT=$(kubectl get deployments,statefulsets,daemonsets,jobs,cronjobs -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
PVC_COUNT=$(kubectl get pvc -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
SVC_COUNT=$(kubectl get svc -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
ING_COUNT=$(kubectl get ingress -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
echo "-> Found ${WL_COUNT} workload controllers, ${POD_COUNT} pods, ${PVC_COUNT} PVCs, ${SVC_COUNT} services, and ${ING_COUNT} ingresses in namespace '${NAMESPACE}'"

echo "-> Deleting namespace '${NAMESPACE}'..."
kubectl delete namespace "${NAMESPACE}" --wait=false

echo ""
echo "✓ Cleanup initiated. Namespace deletion in progress."
echo "Check status: kubectl get all,pvc,ingress -n ${NAMESPACE}"
