# BuilderHub BuildKit Operator

Kubernetes Operator for **BuilderHub** ([builder-hub.dev](https://builder-hub.dev)) — BuildKit as a Service on bare-metal Kubernetes. Supports three builder lifecycles with zero cache loss.

## Architecture

```mermaid
flowchart TB
    subgraph BH["BuilderHub API/Frontend"]
        API[API Server]
        FE[Frontend]
        API --> FE
    end

    subgraph K8S["Kubernetes Cluster"]
        subgraph OP["build-operator"]
            Recon[BuildkitBuilder Controller]
        end

        subgraph EPHEMERAL["ephemeral mode"]
            Pod1[Pod builder-xxx-rand]
            Pod1 --> |ownerReference| CR1[BuildkitBuilder]
        end

        subgraph PERSISTENT["persistent mode"]
            STS[StatefulSet]
            SVC[Headless Service]
            PVC[PVC builder-xxx-cache]
            STS --> SVC
            STS --> PVC
        end

        subgraph SLEEPY["sleepy mode"]
            STS2[StatefulSet 0↔1]
            PVC2[PVC - cache preserved]
            Ann[Annotation last-used]
            STS2 --> PVC2
            Ann --> |idle timeout| STS2
        end
    end

    FE --> |create/scale| Recon
    Recon --> EPHEMERAL
    Recon --> PERSISTENT
    Recon --> SLEEPY
    API --> |PATCH last-used| Ann
```

## Builder Modes

| Mode        | Workload      | Cache                    | Use Case                          |
|-------------|---------------|--------------------------|-----------------------------------|
| **Ephemeral**  | Plain Pod     | emptyDir (or optional PVC) | CI jobs, one build = one pod, auto-cleanup |
| **Persistent** | StatefulSet   | PVC (stable)             | Always-on shared builders         |
| **Sleepy**     | StatefulSet 0↔1 | PVC preserved           | Scale-to-zero, cache instantly restored |

### Sleepy Mode (scale-to-zero)

- BuilderHub API patches `builder.builder-hub.dev/last-used` (RFC3339) when a build starts
- Controller scales StatefulSet to 0 when `now - lastUsed > idleTimeoutSeconds`
- On new build, scale to 1 → same PVC reattaches → cache instantly available

## Quick Start

```bash
# Install CRDs
make install

# Run operator locally
make run

# Deploy to cluster
make docker-build docker-push deploy
```

## Example CRs

### Template (cluster-scoped blueprint)

```yaml
apiVersion: builder-template.builder-hub.dev/v1alpha1
kind: BuildkitBuilderTemplate
metadata:
  name: default-template
spec:
  buildkitImage: moby/buildkit:master-rootless
  rootless: true
  arch: amd64
  cacheConfig:
    type: pvc
    pvc:
      size: "100Gi"
      accessModes: [ReadWriteOnce]
```

### Ephemeral

```yaml
apiVersion: builder-hub.dev/v1alpha1
kind: BuildkitBuilder
metadata:
  name: ephemeral-ci
  namespace: buildkit
spec:
  templateRef: default-template
  mode: ephemeral
  labels:
    builder.builder-hub.dev/pool: ci
```

### Persistent

```yaml
apiVersion: builder-hub.dev/v1alpha1
kind: BuildkitBuilder
metadata:
  name: persistent-shared
  namespace: buildkit
spec:
  templateRef: default-template
  mode: persistent
  replicas: 1
```

### Sleepy

```yaml
apiVersion: builder-hub.dev/v1alpha1
kind: BuildkitBuilder
metadata:
  name: sleepy
  namespace: buildkit
  annotations:
    builder.builder-hub.dev/last-used: "2024-02-22T12:00:00Z"  # Patched by BuilderHub API
spec:
  templateRef: default-template
  mode: sleepy
  idleTimeoutSeconds: 300
```

## BuilderHub Frontend Integration

- Frontend lists `BuildkitBuilder` CRs filtered by `spec.labels`
- Reads `status.endpoint` (e.g. `tcp://10.0.0.1:1234`) for BuildKit connection
- For sleepy: before starting a build, PATCH the CR to set `annotations[builder.builder-hub.dev/last-used]` to current RFC3339 timestamp
- Routes multi-arch builds by `spec.template.arch` → nodeSelector `kubernetes.io/arch`

## Requirements

- Kubernetes 1.28+
- Go 1.23 (for development)
- controller-gen (for `make generate`)

## Project Layout

```
build-operator/
├── api/v1alpha1/           # CRD types
├── cmd/main.go             # Entrypoint
├── internal/controller/    # Reconciliation logic
├── config/                 # Kustomize manifests
│   ├── crd/bases/
│   ├── rbac/
│   ├── manager/
│   └── samples/            # Example CRs
├── helm/build-operator/    # Helm chart
└── Makefile
```

## License

This project is licensed under the [MIT License](LICENSE).
