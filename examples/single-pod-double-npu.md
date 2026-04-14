# Single Pod Requesting Two NPUs

하나의 파드가 NPU 두 개를 동시에 할당받는 예제입니다.

## Apply

```bash
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: Namespace
metadata:
  name: npu-example-double
---
apiVersion: v1
kind: Pod
metadata:
  namespace: npu-example-double
  name: pod0
spec:
  containers:
  - name: ctr0
    image: ubuntu:22.04
    command: ["bash", "-c"]
    args: ["trap 'exit 0' TERM; sleep 9999 & wait"]
    resources:
      limits:
        rebellions.ai/ATOM: 2
      requests:
        rebellions.ai/ATOM: 2
EOF
```

## Verify

```bash
kubectl -n npu-example-double get pod
kubectl -n npu-example-double describe pod pod0
```

## Cleanup

```bash
kubectl delete namespace npu-example-double
```
