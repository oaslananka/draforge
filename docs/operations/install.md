# Installation Guide

DRAForge is provider-neutral and can be deployed on any compatible Kubernetes cluster. The repository provides two primary profiles: a **Demo Profile** for rapid local exploration and a **Production Profile** for robust Helm deployment.

## Demo Profile (Local Development)

The local profile uses the same policy-pinned kind installation exercised by pull-request CI. It creates a Kubernetes v1.35.5 cluster with DRA enabled, installs the hash-verified NetworkPolicy CNI, builds and loads all three local images, installs the complete Helm release, runs the scenario/API verification suite, and keeps the verified cluster for exploration.

### Prerequisites

- Docker with daemon access
- the exact kind version declared in `tests/install-e2e/kubernetes-versions.json`
- kubectl, Helm, jq, curl, Go, Node.js, pnpm, and Task

### Create and keep the verified cluster

```bash
DRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1 task e2e:install-kind
```

The command uses cluster name `draforge-install-e2e`, installs DRAForge in namespace `draforge-system`, and leaves diagnostics under `artifacts/install-e2e/`.

Access the dashboard locally:

```bash
kubectl port-forward svc/draforge-server -n draforge-system 8080:8080
```

Then open `http://localhost:8080`. For CLI/TUI exploration in another terminal:

```bash
task build
./bin/draforge doctor
./bin/draforge tui
```

Delete the disposable cluster when finished:

```bash
kind delete cluster --name draforge-install-e2e
```

## Production Profile (Helm Deployment)

The production profile installs credential-free GHCR images and internal ClusterIP services. External dashboard exposure is disabled by default. The server, controller, and sim-driver repositories are separate, and an empty image tag resolves to the chart `appVersion`.

### Prerequisites
- [Helm v3+](https://helm.sh/docs/intro/install/)
- `kubectl` configured with cluster administrator access.
- A Kubernetes v1.35+ cluster serving `resource.k8s.io/v1`. Core DRA is enabled by default on the supported baseline; distribution-specific configurations must still be verified.

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

   No image pull secret, Gateway, HTTPRoute, or Ingress is rendered by default.

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

### Node Plugin CDI Modes

The default chart uses `nodePlugin.outputMode: demo`. It writes two explicit test devices to a pod-local `emptyDir`, runs as UID 1000, and does not touch the host kubelet CDI directory.

Host-integrated output is an explicit opt-in:

```bash
helm upgrade --install draforge deploy/helm/draforge \
  --namespace draforge-system \
  --create-namespace \
  --set nodePlugin.outputMode=node
```

Node mode mounts `/var/lib/kubelet/device-plugins/cdi` and therefore runs as UID 0 to write the root-owned host directory. It is still non-privileged: privilege escalation is disabled, all capabilities are dropped, the root filesystem is read-only, and RuntimeDefault seccomp remains enabled.

The sim-driver validates that the CDI directory is a writable, non-symlink directory. It writes mode `0644` files through a temporary file, fsync, and atomic rename. Kubernetes API or write failures do not introduce static fallback devices or truncate the current document; the pod becomes not-ready and preserves the last-known-good CDI file until a refresh succeeds. Kubelet probes use `/healthz` and `/readyz` on port `8083`.

### Explicit Local Demo Exposure

For a disposable cluster, the local demo profile enables direct HTTP routing and wildcard CORS:

```bash
helm upgrade --install draforge deploy/helm/draforge \
  --namespace draforge-system \
  --create-namespace \
  -f deploy/helm/draforge/values-local-demo.yaml
```

This profile is unauthenticated and must not be used with sensitive data or production clusters.

### Secure Public Exposure

The chart does not deploy an identity provider. Before using `values-public-example.yaml`, provide:

1. A TLS Secret such as `draforge-example-tls` in the release namespace.
2. An OIDC/identity-aware reverse proxy Service such as `oauth2-proxy:4180` in the same namespace.
3. Proxy upstream configuration pointing to the internal `http://draforge-server:8080` Service.
4. An HTTPS hostname and matching restrictive CORS origin.

```bash
helm upgrade --install draforge deploy/helm/draforge \
  --namespace draforge-system \
  --create-namespace \
  -f deploy/helm/draforge/values-public-example.yaml
```

The secure Gateway route targets the authentication proxy, not the DRAForge Service, preventing the public route from bypassing authentication. The proxy must enforce OIDC login/session policy, forward SSE without buffering, set secure cookies, and add HSTS and other edge headers. CORS only controls browser origin access and does not replace authentication.

For an Ingress controller, set `gateway.enabled: false` and `gateway.ingress.enabled: true` while retaining the same hostname, TLS Secret, proxy Service, and HTTPS CORS origin. Controller-specific annotations may be placed under `gateway.ingress.annotations`.

### Upgrade Note for Current Chart Defaults

Earlier chart defaults created an HTTP Gateway automatically. The current chart creates no external listener unless an explicit profile enables one. Existing public installations must choose either the short-lived local-demo profile or a TLS/authenticated public configuration before upgrading.

### Optional DigitalOcean Showcase Registry Override

This provider-specific showcase is not required for normal DRAForge installations. The optional DOKS showcase build flow publishes component images to a private DigitalOcean Container Registry layout. `task demo:up` creates the `registry-draforge` pull secret in `draforge-system` and installs the chart with:

```bash
helm upgrade --install draforge deploy/helm/draforge \
  --namespace draforge-system \
  --create-namespace \
  -f deploy/helm/draforge/values-showcase-docr.yaml
```

Do not use this override for ordinary public installs; it requires private DOCR credentials.
