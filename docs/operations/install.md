# Installation Guide

DRAForge can be deployed in two primary profiles: a **Demo Profile** for rapid local exploration, and a **Production Profile** for robust deployment on a Kubernetes cluster.

## Demo Profile (Local Development)

The Demo profile is optimized for local exploration using a `kind` cluster with the DRA feature gate enabled. This profile is not suitable for production use as it uses local builds and configurations.

### Prerequisites
- [Go 1.26+](https://go.dev/doc/install)
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/)
- [Task](https://taskfile.dev/installation/)

### Installation Steps

1. **Clone the Repository:**
   ```bash
   git clone https://github.com/oaslananka/draforge.git
   cd draforge
   ```

2. **Build the Binaries:**
   Use Task to compile the binaries into the `bin/` directory.
   ```bash
   task build
   ```

3. **Deploy Core Components:**
   Apply the Custom Resource Definitions (CRDs) and basic scenario manifests to your cluster.
   ```bash
   kubectl apply -f deploy/crds/simulateddevicepool-crd.yaml
   kubectl apply -f examples/scenarios/basic-gpu.yaml
   ```

4. **Verify Installation:**
   You can verify the deployment by launching the Terminal User Interface (TUI).
   ```bash
   ./bin/draforge tui
   ```

## Production Profile (Helm Deployment)

The production profile installs the public chart with credential-free GHCR image defaults. The server, controller, and sim-driver repositories are separate, and an empty image tag resolves to the chart `appVersion`.

### Prerequisites
- [Helm v3+](https://helm.sh/docs/intro/install/)
- `kubectl` configured with cluster administrator access.
- A Kubernetes cluster with the **DynamicResourceAllocation** feature gate enabled.

### Installation Steps

1. **Clone the repository:**
   ```bash
   git clone https://github.com/oaslananka/draforge.git
   cd draforge
   ```

2. **Install via Helm:**
   ```bash
   helm install draforge deploy/helm/draforge \
     --namespace draforge-system \
     --create-namespace
   ```

   The default render uses:
   - `ghcr.io/oaslananka/draforge-server:<appVersion>`
   - `ghcr.io/oaslananka/draforge-controller:<appVersion>`
   - `ghcr.io/oaslananka/draforge-sim-driver:<appVersion>`

   No image pull secret is rendered by default.

3. **Optionally pin immutable image digests:**
   ```bash
   helm upgrade --install draforge deploy/helm/draforge \
     --namespace draforge-system \
     --create-namespace \
     --set-string server.image.digest=sha256:<server-digest> \
     --set-string controller.image.digest=sha256:<controller-digest> \
     --set-string nodePlugin.image.digest=sha256:<sim-driver-digest>
   ```

   A configured digest takes precedence over the component tag.

4. **Verify deployment:**
   ```bash
   kubectl get pods -n draforge-system
   kubectl get svc -n draforge-system
   ```

5. **Access the dashboard for local verification:**
   ```bash
   kubectl port-forward svc/draforge-server -n draforge-system 8080:8080
   ```

   Long-lived public exposure should use an authenticated and TLS-protected ingress path.

### DigitalOcean Showcase Registry Override

The DOKS showcase build flow publishes component images to a private DigitalOcean Container Registry layout. `task demo:up` creates the `registry-draforge` pull secret in `draforge-system` and installs the chart with:

```bash
helm upgrade --install draforge deploy/helm/draforge \
  --namespace draforge-system \
  --create-namespace \
  -f deploy/helm/draforge/values-showcase-docr.yaml
```

Do not use this override for ordinary public installs; it requires private DOCR credentials.
