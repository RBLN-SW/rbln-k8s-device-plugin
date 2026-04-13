#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." >/dev/null 2>&1 && pwd)"

wait_cluster_ready() {
  kubectl get nodes
  kubectl wait --for=condition=Ready nodes --all --timeout=120s
}

detect_resource_name() {
  if [[ -n "${E2E_NPU_RESOURCE_NAME:-}" ]]; then
    return 0
  fi

  if ! command -v jq >/dev/null 2>&1; then
    echo "jq is required to discover the NPU resource name"
    return 1
  fi

  E2E_NPU_RESOURCE_NAME="$(
    kubectl get nodes -o json | jq -r '
      [
        .items[].status.allocatable
        | keys[]
        | select(. == "rebellions.ai/npu" or . == "rebellions.ai/ATOM" or . == "rebellions.ai/REBEL")
      ]
      | unique
      | if index("rebellions.ai/npu") then "rebellions.ai/npu" else .[0] end
    '
  )"

  if [[ -z "${E2E_NPU_RESOURCE_NAME}" || "${E2E_NPU_RESOURCE_NAME}" == "null" ]]; then
    echo "Failed to detect an allocatable Rebellions NPU resource"
    kubectl get nodes -o json | jq '.items[].status.allocatable' || true
    return 1
  fi

  export E2E_NPU_RESOURCE_NAME
  echo "Detected NPU resource name: ${E2E_NPU_RESOURCE_NAME}"
}

wait_pod_phase() {
  local namespace="$1"
  local pod="$2"
  local expected_phase="$3"
  local timeout_seconds="${4:-120}"

  local i
  for ((i=0; i<timeout_seconds; i++)); do
    local phase
    phase="$(kubectl get pod -n "${namespace}" "${pod}" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    if [[ "${phase}" == "${expected_phase}" ]]; then
      return 0
    fi
    sleep 1
  done

  echo "Timed out waiting for pod ${namespace}/${pod} phase=${expected_phase}"
  kubectl get pod -n "${namespace}" "${pod}" -o wide || true
  return 1
}

wait_pod_ready() {
  local namespace="$1"
  local pod="$2"
  kubectl wait --for=condition=Ready -n "${namespace}" "pod/${pod}" --timeout=120s
}

assert_running_pod_count() {
  local namespace="$1"
  local expected="$2"
  local actual
  actual="$(kubectl get pods -n "${namespace}" --no-headers 2>/dev/null | awk '$3 == "Running" {count++} END {print count+0}')"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "Expected ${expected} running pods in namespace ${namespace}, got ${actual}"
    kubectl get pods -n "${namespace}" -o wide || true
    return 1
  fi
}

assert_rbln_smi_group() {
  local namespace="$1"
  local pod="$2"
  local expected_count="$3"
  local container="${4:-}"
  local out
  local stderr_file

  if ! command -v jq >/dev/null 2>&1; then
    echo "jq is required for rbln-smi JSON validation"
    return 1
  fi

  if [[ -z "${container}" ]]; then
    container="$(kubectl get pod -n "${namespace}" "${pod}" -o jsonpath='{.spec.containers[0].name}' 2>/dev/null || true)"
  fi
  if [[ -z "${container}" ]]; then
    echo "Failed to determine container name for ${namespace}/${pod}"
    kubectl get pod -n "${namespace}" "${pod}" -o yaml || true
    return 1
  fi

  stderr_file="$(mktemp)"
  if ! out="$(kubectl exec -n "${namespace}" "${pod}" -c "${container}" -- rbln-smi -j -g 2>"${stderr_file}")"; then
    echo "Failed to execute 'rbln-smi -j -g' in ${namespace}/${pod} (container=${container})"
    cat "${stderr_file}" || true
    rm -f "${stderr_file}"
    return 1
  fi
  rm -f "${stderr_file}"

  if ! jq -e --argjson expected "${expected_count}" '
    .devices as $devices
    | ($devices | type == "array")
      and (($devices | length) == $expected)
      and ($devices | all(has("group_id") and ((.group_id | tostring) | length > 0)))
      and ((($devices | map(.group_id | tostring) | unique) | length) == 1)
  ' >/dev/null <<<"${out}"; then
    echo "rbln-smi validation failed for ${namespace}/${pod} (container=${container})"
    echo "Expected devices=${expected_count} and a single shared group_id"
    echo "${out}" | jq -c '.devices | {count: length, group_ids: (map(.group_id | tostring) | unique)}' || true
    return 1
  fi
}

cleanup_manifest() {
  local manifest="$1"
  kubectl delete -f "${manifest}" --timeout=25s --ignore-not-found=true || true
}

cleanup_namespace() {
  local namespace="$1"
  kubectl delete namespace "${namespace}" --ignore-not-found=true --timeout=60s || true
}
