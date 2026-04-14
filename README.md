# RBLN Device Plugin

`rbln-k8s-device-plugin` is a Kubernetes device plugin for Rebellions NPU devices.
It discovers locally available NPUs, exposes them through the kubelet device plugin
API, and prepares container runtime annotations for CDI-based integration.

The current implementation supports Rebellions device families exposed as:

- `rebellions.ai/ATOM`
- `rebellions.ai/REBEL`
- `rebellions.ai/npu` when generic resource mode is enabled

## Quick Start

### Option 1: Install Through RBLN NPU Operator

If your cluster is managed through the RBLN NPU Operator, install the operator first:

```bash
helm repo add rebellions https://rbln-sw.github.io/rbln-npu-operator
helm repo update

helm install --wait --generate-name \
  -n rbln-system --create-namespace \
  rebellions/rbln-npu-operator
```

### Option 2: Install This Device Plugin Chart Directly

1. Build and publish the image:

```bash
make -f deployments/container/Makefile build \
  IMAGE_NAME=<registry>/k8s-device-plugin \
  VERSION=<tag>

make -f deployments/container/Makefile push \
  IMAGE_NAME=<registry>/k8s-device-plugin \
  VERSION=<tag>
```

2. Install the Helm chart from this repository:

```bash
helm upgrade --install rbln-device-plugin \
  ./deployments/helm/rbln-device-plugin \
  -n rbln-device-plugin \
  --create-namespace \
  --set image.repository=<registry>/k8s-device-plugin \
  --set image.tag=<tag>
```

3. Verify the rollout:

```bash
kubectl -n rbln-device-plugin get daemonset,pods
kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.allocatable}{"\n"}{end}'
```

If generic resource mode is enabled, you should see `rebellions.ai/npu`.
Otherwise, allocatable resources are exposed as `rebellions.ai/ATOM` and/or
`rebellions.ai/REBEL` depending on installed hardware.

## Configuration

The binary can be configured with CLI flags or environment variables.

| Flag | Environment variable | Default | Description |
| --- | --- | --- | --- |
| `--cdi-root` | `CDI_ROOT` | `/var/run/cdi` | Directory used for CDI spec management |
| `--kubelet-device-plugin-path` | `KUBELET_DEVICE_PLUGIN_PATH` | `/var/lib/kubelet/device-plugins` | Kubelet device plugin socket directory |
| `--healthcheck-port` | `HEALTHCHECK_PORT` | `51515` | gRPC healthcheck port; set a negative value to disable it |
| `--use-generic-resource-name` | `USE_GENERIC_RESOURCE_NAME` | `false` | Expose `rebellions.ai/npu` instead of per-product resources |
| `--device-scan-interval` | `DEVICE_SCAN_INTERVAL` | `1m` | Polling interval for refreshing the device inventory |

## License

This project is licensed under the Apache License 2.0. See
[`LICENSE`](LICENSE) for details.
