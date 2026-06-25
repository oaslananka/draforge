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

The Production profile is designed for deploying DRAForge into a remote Kubernetes cluster (e.g., DigitalOcean Kubernetes). This method uses Helm to manage the application lifecycle.

### Prerequisites
- [Helm v3+](https://helm.sh/docs/intro/install/)
- `kubectl` configured with cluster administrator access.
- A Kubernetes cluster with the **DynamicResourceAllocation** feature gate enabled.

### Installation Steps

1. **Add the Repository (If applicable):**
   *(Note: Adjust this step based on where the Helm chart is hosted, or use the local chart directory if cloning from source)*
   ```bash
   # From the cloned repository root:
   cd deploy/helm/draforge
   ```

2. **Install via Helm:**
   Deploy the DRAForge stack into a dedicated namespace (`draforge-system`).
   ```bash
   helm install draforge deploy/helm/draforge \
     --namespace draforge-system \
     --create-namespace
   ```

3. **Verify Deployment:**
   Check the status of the deployed pods and services:
   ```bash
   kubectl get pods -n draforge-system
   kubectl get svc -n draforge-system
   ```

4. **Access the Dashboard (Optional):**
   In production, the dashboard should be exposed securely. For quick verification, you can port-forward the API server:
   ```bash
   kubectl port-forward svc/draforge-api-server -n draforge-system 8080:80
   ```
   *Note: Ensure proper ingress and authentication are configured for long-term access, as the dashboard is unprotected by default.*