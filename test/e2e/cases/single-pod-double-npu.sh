#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

NAMESPACE="npu-single-pod-double-npu"
trap 'cleanup_namespace "${NAMESPACE}"' EXIT

wait_cluster_ready
detect_resource_name

echo "=== single-pod-double-npu: one pod requests two NPU devices ==="
kubectl apply -f - <<EOF_MANIFEST
apiVersion: v1
kind: Namespace
metadata:
  name: ${NAMESPACE}
---
apiVersion: v1
kind: Pod
metadata:
  namespace: ${NAMESPACE}
  name: pod0
spec:
  containers:
  - name: ctr0
    image: ubuntu:22.04
    command: ["bash", "-c"]
    args: ["export; trap 'exit 0' TERM; sleep 9999 & wait"]
    resources:
      limits:
        ${E2E_NPU_RESOURCE_NAME}: 2
      requests:
        ${E2E_NPU_RESOURCE_NAME}: 2
EOF_MANIFEST

wait_pod_ready "${NAMESPACE}" pod0
assert_running_pod_count "${NAMESPACE}" 1
assert_rbln_smi_group "${NAMESPACE}" pod0 2
