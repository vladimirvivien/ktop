#!/bin/bash
#
# Generate a kubeconfig file for ktop test users
#
# Usage:
#   ./generate-kubeconfig.sh <user>
#
# Examples:
#   ./generate-kubeconfig.sh ktop-full
#   ./generate-kubeconfig.sh ktop-restricted
#

set -e

USER=${1:-}

if [ -z "$USER" ]; then
    echo "Usage: $0 <user>"
    echo ""
    echo "Available users:"
    echo "  ktop-full       - Full access (prometheus + metrics-server)"
    echo "  ktop-restricted - Restricted access (metrics-server only)"
    exit 1
fi

NAMESPACE="ktop-test"
OUTPUT_FILE="/tmp/ktop-${USER}.kubeconfig"

# Verify the service account exists
if ! kubectl get serviceaccount "$USER" -n "$NAMESPACE" &>/dev/null; then
    echo "Error: ServiceAccount '$USER' not found in namespace '$NAMESPACE'"
    echo ""
    echo "Did you apply the RBAC configuration?"
    echo "  kubectl apply -f ktop-user.yaml"
    exit 1
fi

echo "Generating kubeconfig for $USER..."

# Generate token (valid for 24 hours)
TOKEN=$(kubectl create token "$USER" -n "$NAMESPACE" --duration=24h)

# Get cluster info
CLUSTER_SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')

# Try to get embedded CA data first, fall back to reading from file (minikube)
CLUSTER_CA=$(kubectl config view --minify --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
if [ -z "$CLUSTER_CA" ]; then
    CA_FILE=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.certificate-authority}')
    if [ -n "$CA_FILE" ] && [ -f "$CA_FILE" ]; then
        CLUSTER_CA=$(base64 -w 0 < "$CA_FILE")
    else
        echo "Error: Could not find cluster CA certificate"
        exit 1
    fi
fi

# Create kubeconfig
cat > "$OUTPUT_FILE" <<EOF
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

echo "Created: $OUTPUT_FILE"
echo ""
echo "Test with:"
echo "  ktop --kubeconfig=$OUTPUT_FILE"
