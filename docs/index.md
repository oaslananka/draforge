---
title: DRAForge
layout: default
description: Diagnose Kubernetes ResourceClaim allocation, explore simulated devices, and observe DRA state with a read-only dashboard.
---

<link rel="stylesheet" href="{{ '/assets/css/landing.css' | relative_url }}">

<div class="df-home">
  <header class="df-hero" aria-labelledby="df-hero-title">
    <p class="df-eyebrow">Kubernetes Dynamic Resource Allocation · Operator toolkit</p>
    <h2 id="df-hero-title">Find out why a ResourceClaim is pending.</h2>
    <p class="df-hero-description">Inspect device pools, follow allocation decisions, and test a fix against simulated accelerators. Run locally without a physical GPU or connect to your existing compatible Kubernetes cluster.</p>
    <div class="df-actions">
      <a class="df-action df-action-primary" href="#try-locally">Try it with local kind</a>
      <a class="df-action df-action-secondary" href="#existing-cluster">Use an existing cluster</a>
    </div>
    <dl class="df-facts">
      <div><dt>Hardware</dt><dd>No GPU required for simulation</dd></div>
      <div><dt>Dashboard access</dt><dd>Read-only, internal by default</dd></div>
      <div><dt>Kubernetes baseline</dt><dd>v1.35+ · <code>resource.k8s.io/v1</code></dd></div>
    </dl>
  </header>

  <section class="df-section" aria-labelledby="df-paths-title">
    <p class="df-section-label">FIRST RUN</p>
    <h2 id="df-paths-title">Choose the environment you have</h2>
    <p class="df-section-intro">Both paths use the same DRAForge diagnostics. Only the local path creates a disposable kind cluster and simulated devices.</p>

    <div class="df-path-grid">
      <article class="df-path df-path-highlight" id="try-locally" aria-labelledby="df-local-title">
        <div class="df-path-topline"><span>01 · Local simulation</span><span>No cloud account</span></div>
        <h3 id="df-local-title">Explore a complete DRA stack</h3>
        <p>Build and install the simulator, controller, server, and example claims in a new kind cluster. The installer runs real Kubernetes checks before leaving it available for you.</p>
        <p class="df-prereqs"><strong>You need:</strong> Bash or WSL, Docker accessible from that shell (verify with <code>docker info</code>), pinned kind, kubectl, Helm, jq, curl and sha256sum. On Windows, enable Docker Desktop's WSL integration for your distribution. Go, Node.js and pnpm are not required on the host for this stack.</p>
        <p class="df-command-label">In your terminal</p>
        <pre><code>git clone https://github.com/oaslananka/draforge.git
cd draforge
DRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1 bash scripts/kind-install-e2e.sh pull-request</code></pre>
        <p class="df-command-label">Then, in another terminal</p>
        <pre><code>kubectl port-forward -n draforge-system svc/draforge-server 8080:8080</code></pre>
        <p>Open <a href="http://localhost:8080"><code>http://localhost:8080</code></a> on the same machine. Check Claims, Graph, and Explain to investigate the simulated allocation.</p>
        <p class="df-guardrail"><strong>Existing cluster?</strong> The installer refuses to replace it. Inspect the old cluster or choose a new name with <code>DRAFORGE_INSTALL_E2E_CLUSTER_NAME</code>.</p>
        <a class="df-more" href="{{ '/operations/install.html#create-and-keep-the-verified-cluster' | relative_url }}">Local setup, requirements and cleanup</a>
      </article>

      <article class="df-path" id="existing-cluster" aria-labelledby="df-existing-title">
        <div class="df-path-topline"><span>02 · Kubernetes</span><span>Bring your cluster</span></div>
        <h3 id="df-existing-title">Observe an existing DRA environment</h3>
        <p>Install DRAForge via Helm in a compatible cluster. The normal install keeps the dashboard inside the cluster; you choose how and when to access it.</p>
        <p class="df-prereqs"><strong>You need:</strong> Kubernetes v1.35+ serving <code>resource.k8s.io/v1</code>, an authorized kubectl context, Helm 3, and access to the public component images.</p>
        <p class="df-command-label">From a clone of the repository</p>
        <pre><code>helm upgrade --install draforge deploy/helm/draforge \
  --namespace draforge-system \
  --create-namespace</code></pre>
        <p class="df-command-label">View the dashboard locally</p>
        <pre><code>kubectl port-forward -n draforge-system svc/draforge-server 8080:8080</code></pre>
        <p>Open <a href="http://localhost:8080"><code>http://localhost:8080</code></a>. No simulated claim is created by the standard Helm install; it displays the DRA state already present in your cluster.</p>
        <p class="df-guardrail"><strong>Read-only by design.</strong> The dashboard shows cluster metadata but cannot create, fix, or delete Kubernetes resources. Keep public access behind TLS and authentication.</p>
        <a class="df-more" href="{{ '/operations/install.html#production-profile-helm-deployment' | relative_url }}">Helm installation and secure access</a>
      </article>
    </div>
  </section>

  <section class="df-section df-observe" aria-labelledby="df-observe-title">
    <p class="df-section-label">WHAT YOU'LL BE ABLE TO VERIFY</p>
    <h2 id="df-observe-title">From a pending claim to a diagnosis</h2>
    <div class="df-outcomes">
      <div><span class="df-outcome-number" aria-hidden="true">1</span><h3>Inspect</h3><p>Review device pools, ResourceSlices and ResourceClaims in the dashboard or CLI.</p></div>
      <div><span class="df-outcome-number" aria-hidden="true">2</span><h3>Explain</h3><p>Identify missing DeviceClasses, selector mismatch, or capacity and node constraints.</p></div>
      <div><span class="df-outcome-number" aria-hidden="true">3</span><h3>Observe recovery</h3><p>After an authorized operator-side repair, see the updated claim in Graph, Claims and Explain without reloading the page.</p></div>
    </div>
    <p class="df-evidence">This recovery journey is exercised against a real kind Kubernetes API and SSE stream using simulated devices. It does not certify physical GPU scheduling or vendor hardware.</p>
    <a class="df-more" href="{{ '/e2e-matrix.html' | relative_url }}">See what the install test actually verifies</a>
  </section>

  <section class="df-section" aria-labelledby="df-docs-title">
    <p class="df-section-label">REFERENCE</p>
    <h2 id="df-docs-title">Go directly to the right guide</h2>
    <div class="df-link-grid">
      <a href="{{ '/dashboard.html' | relative_url }}"><strong>Dashboard</strong><span>Views, live stream and read-only limits</span></a>
      <a href="{{ '/operations/troubleshooting.html' | relative_url }}"><strong>Troubleshooting</strong><span>Diagnose pending claims and runtime failures</span></a>
      <a href="{{ '/compatibility/kubernetes-dra.html' | relative_url }}"><strong>Compatibility</strong><span>Supported DRA APIs and Kubernetes versions</span></a>
      <a href="{{ '/simulator-scenarios.html' | relative_url }}"><strong>Simulation</strong><span>Device pool scenarios and test inputs</span></a>
      <a href="{{ '/architecture.html' | relative_url }}"><strong>Architecture</strong><span>How the server, controller and plugin work</span></a>
      <a href="{{ '/operations/guide.html' | relative_url }}"><strong>Operations</strong><span>Day-two verification, logs and cleanup</span></a>
    </div>
  </section>

  <footer class="df-footer">
    <p>DRAForge is open source and provider-neutral. The hosted site is documentation, <strong>not a running dashboard connected to a public cluster</strong>.</p>
    <nav aria-label="Project resources">
      <a href="https://github.com/oaslananka/draforge">Source code</a>
      <a href="https://github.com/oaslananka/draforge/releases/latest">Releases</a>
      <a href="https://github.com/oaslananka/draforge/blob/main/SECURITY.md">Security</a>
      <a href="https://github.com/oaslananka/draforge/blob/main/CONTRIBUTING.md">Contribute</a>
    </nav>
  </footer>
</div>
