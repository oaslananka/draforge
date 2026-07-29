#!/usr/bin/env python3
"""Validate public documentation against repository-owned runtime contracts."""

from __future__ import annotations

import re
import shutil
import sys
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
FIXTURE_ROOT = ROOT / "artifacts" / "documentation-contract-fixture"

README_PATH = Path("README.md")
CONTRIBUTING_PATH = Path("CONTRIBUTING.md")
SECURITY_PATH = Path("SECURITY.md")
GOVERNANCE_PATH = Path("GOVERNANCE.md")
CHANGELOG_PATH = Path("CHANGELOG.md")
TASKFILE_PATH = Path("Taskfile.yml")
COMPATIBILITY_PATH = Path("docs/compatibility/kubernetes-dra.md")
INSTALL_PATH = Path("docs/operations/install.md")
E2E_DOC_PATH = Path("docs/e2e-matrix.md")
DEMO_PATH = Path("docs/demo.md")
CHART_PATH = Path("deploy/helm/draforge/Chart.yaml")
SERVER_SERVICE_PATH = Path("deploy/helm/draforge/templates/service-server.yaml")
VALUES_PATH = Path("deploy/helm/draforge/values.yaml")
CLI_SOURCE_PATH = Path("cmd/draforge/main.go")
MATRIX_WORKFLOW_PATH = Path(".github/workflows/e2e-matrix.yml")
PORTABLE_WORKFLOW_PATH = Path(".github/workflows/e2e-kubernetes.yml")
DOKS_WORKFLOW_PATH = Path(".github/workflows/e2e.yml")
RELEASE_WORKFLOW_PATH = Path(".github/workflows/release.yml")
SCORECARD_WORKFLOW_PATH = Path(".github/workflows/scorecard.yml")

ACTIVE_PATH_PREFIXES = (
    "scripts/",
    "deploy/",
    "examples/",
    "tests/",
    ".github/workflows/",
    "infra/terraform/",
)
PATH_CHARACTERS = frozenset(
    "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._/:-"
)
LINK_PATTERN = re.compile(r"\]\(([^)\n]+)\)")
SEMVER_PATTERN = re.compile(r"(?:0|[1-9]\d*)(?:\.(?:0|[1-9]\d*)){2}")
CLI_USE_PATTERN = re.compile(r'Use:\s+"([a-z][a-z-]*)')


def read_text(root: Path, relative: Path, errors: list[str]) -> str:
    path = root / relative
    try:
        return path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as exc:
        errors.append(f"cannot read {relative}: {exc}")
        return ""


def markdown_files(root: Path) -> list[Path]:
    files = [path.relative_to(root) for path in sorted(root.glob("*.md"))]
    docs_root = root / "docs"
    if docs_root.is_dir():
        files.extend(path.relative_to(root) for path in sorted(docs_root.rglob("*.md")))
    return files


def active_markdown_files(root: Path) -> list[Path]:
    return [
        path
        for path in markdown_files(root)
        if path != CHANGELOG_PATH and Path("docs/adr") not in path.parents
    ]


def normalize_link_target(target: str) -> str:
    cleaned = target.strip().strip("<>")
    cleaned = cleaned.split("#", 1)[0].split("?", 1)[0]
    return unquote(cleaned)


def validate_markdown_links(root: Path, files: list[Path]) -> list[str]:
    errors: list[str] = []
    repository_root = root.resolve()
    for relative in files:
        text = read_text(root, relative, errors)
        if text.count("```") % 2:
            errors.append(f"{relative}: unbalanced fenced code blocks")
        for match in LINK_PATTERN.finditer(text):
            target = normalize_link_target(match.group(1))
            if not target or urlsplit(target).scheme:
                continue
            resolved = (root / relative.parent / target).resolve()
            if not resolved.is_relative_to(repository_root):
                errors.append(f"{relative}: relative link escapes repository: {target}")
            elif not resolved.exists():
                errors.append(f"{relative}: missing relative link target: {target}")
    return errors


def repository_references(text: str) -> set[str]:
    references: set[str] = set()
    for prefix in ACTIVE_PATH_PREFIXES:
        offset = 0
        while True:
            start = text.find(prefix, offset)
            if start < 0:
                break
            end = start + len(prefix)
            while end < len(text) and text[end] in PATH_CHARACTERS:
                end += 1
            reference = text[start:end].rstrip(".,;:)")
            if reference.endswith("/..."):
                reference = reference[:-4]
            references.add(reference)
            offset = end
    return references


def validate_repository_paths(root: Path, files: list[Path]) -> list[str]:
    errors: list[str] = []
    for relative in files:
        text = read_text(root, relative, errors)
        for reference in sorted(repository_references(text)):
            if not (root / reference).exists():
                errors.append(
                    f"{relative}: referenced repository path does not exist: {reference}"
                )
    return errors


def task_names(taskfile: str) -> set[str]:
    names: set[str] = set()
    for line in taskfile.splitlines():
        if not line.startswith("  ") or line.startswith("    "):
            continue
        stripped = line.strip()
        if stripped.endswith(":"):
            name = stripped[:-1]
            if name and all(character.isalnum() or character in ":_-" for character in name):
                names.add(name)
    return names


def inline_code_spans(text: str) -> list[str]:
    parts = text.split("`")
    return parts[1::2]


def referenced_tasks(text: str) -> set[str]:
    references: set[str] = set()
    for span in inline_code_spans(text):
        if span.startswith("task "):
            references.add(span.split()[1])
    for line in text.splitlines():
        stripped = line.strip()
        if stripped.startswith("task "):
            references.add(stripped.split()[1])
        marker = "# or: task "
        if marker in stripped:
            references.add(stripped.split(marker, 1)[1].split()[0])
    return references


def validate_tasks(root: Path, files: list[Path]) -> list[str]:
    errors: list[str] = []
    available = task_names(read_text(root, TASKFILE_PATH, errors))
    for relative in files:
        text = read_text(root, relative, errors)
        for name in sorted(referenced_tasks(text) - available):
            errors.append(f"{relative}: referenced task does not exist: {name}")
    return errors


def chart_fields(chart: str) -> dict[str, str]:
    fields: dict[str, str] = {}
    for line in chart.splitlines():
        key, separator, value = line.partition(":")
        if separator and key in {"version", "appVersion"}:
            fields[key] = value.strip().strip('"')
    return fields


def chart_version(root: Path, errors: list[str]) -> str:
    fields = chart_fields(read_text(root, CHART_PATH, errors))
    version = fields.get("version", "")
    app_version = fields.get("appVersion", "")
    if not version or version != app_version:
        errors.append(f"{CHART_PATH}: version and appVersion must match")
        return ""
    if SEMVER_PATTERN.fullmatch(version) is None:
        errors.append(f"Chart version is not stable semantic version metadata: {version}")
        return ""
    return version


def validate_version_contract(root: Path) -> list[str]:
    errors: list[str] = []
    version = chart_version(root, errors)
    if not version:
        return errors
    major, minor, _ = version.split(".")
    support_line = f"| v{major}.{minor}.x  | Yes       |"
    unsupported_line = f"| < v{major}.{minor}  | No        |"
    security = read_text(root, SECURITY_PATH, errors)
    if support_line not in security:
        errors.append(f"{SECURITY_PATH} must declare current support row: {support_line}")
    if unsupported_line not in security:
        errors.append(
            f"{SECURITY_PATH} must declare unsupported row: {unsupported_line}"
        )

    compatibility = read_text(root, COMPATIBILITY_PATH, errors)
    fragments = (
        f"> **DRAForge version:** {version}",
        f"| Capability | Upstream status as of v1.36 docs | DRAForge {version} support |",
    )
    errors.extend(require_fragments(COMPATIBILITY_PATH, compatibility, fragments))

    stale_patterns = {
        "DRAForge 0.1.0 support": "stale compatibility release label",
        "| v0.1.x  | Yes": "stale supported-version row",
        "| < v0.1  | No": "stale unsupported-version row",
    }
    for relative in active_markdown_files(root):
        text = read_text(root, relative, errors)
        for pattern, description in stale_patterns.items():
            if pattern in text:
                errors.append(f"{relative}: {description}: {pattern}")
    return errors


def require_fragments(
    relative: Path, text: str, fragments: tuple[str, ...]
) -> list[str]:
    return [
        f"{relative}: missing documentation contract fragment: {fragment}"
        for fragment in fragments
        if fragment not in text
    ]


def validate_e2e_workflows(root: Path) -> list[str]:
    errors: list[str] = []
    matrix = read_text(root, MATRIX_WORKFLOW_PATH, errors)
    portable = read_text(root, PORTABLE_WORKFLOW_PATH, errors)
    doks = read_text(root, DOKS_WORKFLOW_PATH, errors)
    release = read_text(root, RELEASE_WORKFLOW_PATH, errors)
    errors.extend(
        require_fragments(
            MATRIX_WORKFLOW_PATH,
            matrix,
            ("pull_request:", "schedule:", "workflow_dispatch:", "workflow_call:"),
        )
    )
    errors.extend(
        require_fragments(
            PORTABLE_WORKFLOW_PATH,
            portable,
            ("pull_request:", "workflow_dispatch:", "environment: e2e-kubernetes"),
        )
    )
    if "schedule:" in doks:
        errors.append(f"{DOKS_WORKFLOW_PATH}: optional DOKS adapter must be manual-only")
    errors.extend(
        require_fragments(
            RELEASE_WORKFLOW_PATH,
            release,
            (f"uses: ./{MATRIX_WORKFLOW_PATH}", "profile: full"),
        )
    )
    return errors


def validate_e2e_documentation(root: Path) -> list[str]:
    errors: list[str] = []
    matrix_doc = read_text(root, E2E_DOC_PATH, errors)
    matrix_fragments = (
        "Kubernetes v1.35.5",
        "Kubernetes v1.35.5 and v1.36.1",
        "weekly schedule",
        "manual full runs",
        "tagged releases",
        str(PORTABLE_WORKFLOW_PATH),
        "KUBECONFIG_B64",
        "Doppler",
        "manual `Optional DOKS E2E` workflow",
        "does not create or destroy the cluster",
    )
    errors.extend(require_fragments(E2E_DOC_PATH, matrix_doc, matrix_fragments))

    security = read_text(root, SECURITY_PATH, errors)
    security_fragments = (
        str(MATRIX_WORKFLOW_PATH),
        "weekly",
        str(DOKS_WORKFLOW_PATH),
        "manual-only",
        "Doppler",
    )
    errors.extend(require_fragments(SECURITY_PATH, security, security_fragments))
    if "nightly schedule" in security:
        errors.append(
            f"{SECURITY_PATH}: optional DOKS workflow is incorrectly described as nightly"
        )
    return errors


def validate_e2e_contract(root: Path) -> list[str]:
    return validate_e2e_workflows(root) + validate_e2e_documentation(root)


def documented_cli_commands(text: str) -> set[str]:
    commands: set[str] = set()
    for line in text.splitlines():
        stripped = line.strip()
        if stripped.startswith("./bin/draforge "):
            commands.add(stripped.split()[1])
        elif stripped.startswith("draforge "):
            commands.add(stripped.split()[1])
    return commands


def validate_cli_contract(root: Path, files: list[Path]) -> list[str]:
    errors: list[str] = []
    source = read_text(root, CLI_SOURCE_PATH, errors)
    available = set(CLI_USE_PATTERN.findall(source))
    available.discard("draforge")
    for relative in files:
        text = read_text(root, relative, errors)
        for command in sorted(documented_cli_commands(text) - available):
            errors.append(f"{relative}: documented CLI command does not exist: {command}")
    return errors


def validate_install_contract(root: Path) -> list[str]:
    errors: list[str] = []
    install = read_text(root, INSTALL_PATH, errors)
    readme = read_text(root, README_PATH, errors)
    contributing = read_text(root, CONTRIBUTING_PATH, errors)
    compatibility = read_text(root, COMPATIBILITY_PATH, errors)

    errors.extend(
        require_fragments(
            INSTALL_PATH,
            install,
            (
                "DRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1 task e2e:install-kind",
                "kubectl port-forward svc/draforge-server -n draforge-system 8080:8080",
                "kind delete cluster --name draforge-install-e2e",
                "helm install draforge deploy/helm/draforge",
                "No image pull secret, Gateway, HTTPRoute, or Ingress is rendered by default.",
            ),
        )
    )
    errors.extend(
        require_fragments(
            README_PATH,
            readme,
            (
                "DRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1 task e2e:install-kind",
                "kubectl port-forward svc/draforge-server -n draforge-system 8080:8080",
                "kind delete cluster --name draforge-install-e2e",
            ),
        )
    )
    errors.extend(
        require_fragments(
            CONTRIBUTING_PATH,
            contributing,
            ("Kubernetes v1.35+", "resource.k8s.io/v1", "helm upgrade --install draforge"),
        )
    )
    if "| RBAC least privilege | Supported |" not in compatibility:
        errors.append(
            f"{COMPATIBILITY_PATH}: RBAC support status must match the verified chart contract"
        )
    return errors


def top_level_yaml_section(text: str, name: str) -> list[str]:
    lines = text.splitlines()
    start = next((index for index, line in enumerate(lines) if line == f"{name}:"), -1)
    if start < 0:
        return []
    section: list[str] = []
    for line in lines[start + 1 :]:
        if line and not line.startswith(" "):
            break
        section.append(line)
    return section


def nested_yaml_value(lines: list[str], section_name: str, key: str) -> str | None:
    in_section = False
    for line in lines:
        if line == f"  {section_name}:":
            in_section = True
            continue
        if in_section and line.startswith("  ") and not line.startswith("    "):
            return None
        if in_section and line.startswith(f"    {key}:"):
            return line.partition(":")[2].strip()
    return None


def validate_helm_service_contract(root: Path) -> list[str]:
    errors: list[str] = []
    template = read_text(root, SERVER_SERVICE_PATH, errors)
    errors.extend(
        require_fragments(
            SERVER_SERVICE_PATH,
            template,
            (
                'name: {{ include "draforge.fullname" . }}-server',
                'port: {{ .Values.server.service.port }}',
                "targetPort: 8080",
            ),
        )
    )
    values = read_text(root, VALUES_PATH, errors)
    server_lines = top_level_yaml_section(values, "server")
    if not server_lines:
        errors.append(f"{VALUES_PATH}: server values section is missing")
    elif nested_yaml_value(server_lines, "service", "port") != "8080":
        errors.append(f"{VALUES_PATH}: server Service port must remain 8080")
    return errors


def validate_security_accuracy(root: Path) -> list[str]:
    errors: list[str] = []
    security = read_text(root, SECURITY_PATH, errors)
    forbidden = (
        "configured with Dependabot for dependency updates",
        "Dependabot security updates | Auto-merge dependency fixes",
    )
    for phrase in forbidden:
        if phrase in security:
            errors.append(
                f"{SECURITY_PATH} contains an unsupported repository-setting claim: {phrase}"
            )
    return errors


def validate_governance_continuity(root: Path) -> list[str]:
    errors: list[str] = []
    governance = read_text(root, GOVERNANCE_PATH, errors)
    errors.extend(
        require_fragments(
            GOVERNANCE_PATH,
            governance,
            (
                "single-human-maintainer project",
                "solo-maintainer succession mechanism",
                "designated executor",
                "private encrypted succession package",
                "legal authority",
                "within one week",
                "`bus_factor` SHOULD",
                "does not block Silver",
                "issues/144",
            ),
        )
    )
    forbidden = (
        "second qualified human",
        "second maintainer",
        "two valid ways",
        "Path A",
    )
    for phrase in forbidden:
        if phrase.lower() in governance.lower():
            errors.append(
                f"{GOVERNANCE_PATH}: second-maintainer continuity path is not allowed: {phrase}"
            )
    return errors


def validate_scorecard_contract(root: Path) -> list[str]:
    errors: list[str] = []
    readme = read_text(root, README_PATH, errors)
    workflow = read_text(root, SCORECARD_WORKFLOW_PATH, errors)
    errors.extend(
        require_fragments(
            README_PATH,
            readme,
            (
                "https://api.scorecard.dev/projects/github.com/oaslananka/draforge/badge",
                "https://scorecard.dev/viewer/?uri=github.com/oaslananka/draforge",
            ),
        )
    )
    if "securityscorecards.dev" in readme:
        errors.append(f"{README_PATH}: legacy OpenSSF Scorecard domain is not allowed")
    errors.extend(
        require_fragments(
            SCORECARD_WORKFLOW_PATH,
            workflow,
            (
                "contents: read",
                "issues: read",
                "pull-requests: read",
                "checks: read",
                "id-token: write",
                "security-events: write",
                "persist-credentials: false",
                "ossf/scorecard-action@2d1146689b8cda280b9bc96326124645441f03bc",
                "publish_results: true",
                "github/codeql-action/upload-sarif@",
            ),
        )
    )
    if any(line == "env:" for line in workflow.splitlines()):
        errors.append(
            f"{SCORECARD_WORKFLOW_PATH}: top-level env is incompatible with published Scorecard results"
        )
    return errors


def validate_best_practices_badge_contract(root: Path) -> list[str]:
    errors: list[str] = []
    readme = read_text(root, README_PATH, errors)
    errors.extend(
        require_fragments(
            README_PATH,
            readme,
            (
                "https://www.bestpractices.dev/projects/13404/badge",
                "https://www.bestpractices.dev/projects/13404",
            ),
        )
    )
    return errors


def validate_root(root: Path) -> list[str]:
    files = markdown_files(root)
    active_files = active_markdown_files(root)
    errors: list[str] = []
    errors.extend(validate_markdown_links(root, files))
    errors.extend(validate_repository_paths(root, active_files))
    errors.extend(validate_tasks(root, active_files))
    errors.extend(validate_version_contract(root))
    errors.extend(validate_e2e_contract(root))
    errors.extend(validate_cli_contract(root, active_files))
    errors.extend(validate_install_contract(root))
    errors.extend(validate_helm_service_contract(root))
    errors.extend(validate_security_accuracy(root))
    errors.extend(validate_governance_continuity(root))
    errors.extend(validate_scorecard_contract(root))
    errors.extend(validate_best_practices_badge_contract(root))
    return errors


def write_fixture(relative: Path, content: str) -> None:
    path = FIXTURE_ROOT / relative
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def reset_fixture() -> None:
    shutil.rmtree(FIXTURE_ROOT, ignore_errors=True)
    files = {
        README_PATH: f"[Install]({INSTALL_PATH})\nDRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1 task e2e:install-kind\nkubectl port-forward svc/draforge-server -n draforge-system 8080:8080\nkind delete cluster --name draforge-install-e2e\nhttps://api.scorecard.dev/projects/github.com/oaslananka/draforge/badge\nhttps://scorecard.dev/viewer/?uri=github.com/oaslananka/draforge\nhttps://www.bestpractices.dev/projects/13404/badge\nhttps://www.bestpractices.dev/projects/13404\n",
        CONTRIBUTING_PATH: "Kubernetes v1.35+ serves resource.k8s.io/v1.\nhelm upgrade --install draforge deploy/helm/draforge\n",
        SECURITY_PATH: f"| v0.2.x  | Yes       |\n| < v0.2  | No        |\n{MATRIX_WORKFLOW_PATH} runs weekly. {DOKS_WORKFLOW_PATH} is manual-only. Doppler is the secret source.\n",
        GOVERNANCE_PATH: "single-human-maintainer project\nsolo-maintainer succession mechanism\ndesignated executor\nprivate encrypted succession package\nlegal authority\nwithin one week\n`bus_factor` SHOULD\ndoes not block Silver\nhttps://github.com/oaslananka/draforge/issues/144\n",
        CHANGELOG_PATH: "# Changelog\n",
        TASKFILE_PATH: "tasks:\n  e2e:install-kind:\n    cmds: []\n",
        CHART_PATH: 'version: 0.2.0\nappVersion: "0.2.0"\n',
        SERVER_SERVICE_PATH: 'name: {{ include "draforge.fullname" . }}-server\nport: {{ .Values.server.service.port }}\ntargetPort: 8080\n',
        VALUES_PATH: "server:\n  service:\n    type: ClusterIP\n    port: 8080\ncontroller:\n  enabled: true\n",
        COMPATIBILITY_PATH: "> **DRAForge version:** 0.2.0\n| Capability | Upstream status as of v1.36 docs | DRAForge 0.2.0 support |\n| RBAC least privilege | Supported | Verified |\n",
        INSTALL_PATH: "DRAFORGE_INSTALL_E2E_KEEP_CLUSTER=1 task e2e:install-kind\nkubectl port-forward svc/draforge-server -n draforge-system 8080:8080\nkind delete cluster --name draforge-install-e2e\nhelm install draforge deploy/helm/draforge\nNo image pull secret, Gateway, HTTPRoute, or Ingress is rendered by default.\n",
        E2E_DOC_PATH: f"Kubernetes v1.35.5\nKubernetes v1.35.5 and v1.36.1\nweekly schedule\nmanual full runs\ntagged releases\n{PORTABLE_WORKFLOW_PATH}\nKUBECONFIG_B64\nDoppler\nmanual `Optional DOKS E2E` workflow\ndoes not create or destroy the cluster\n",
        MATRIX_WORKFLOW_PATH: "on:\n  pull_request:\n  schedule:\n  workflow_dispatch:\n  workflow_call:\n",
        PORTABLE_WORKFLOW_PATH: "on:\n  pull_request:\n  workflow_dispatch:\njobs:\n  external:\n    environment: e2e-kubernetes\n",
        DOKS_WORKFLOW_PATH: "on:\n  workflow_dispatch:\n",
        RELEASE_WORKFLOW_PATH: f"uses: ./{MATRIX_WORKFLOW_PATH}\nprofile: full\n",
        SCORECARD_WORKFLOW_PATH: "permissions:\n  contents: read\njobs:\n  scorecard:\n    permissions:\n      contents: read\n      issues: read\n      pull-requests: read\n      checks: read\n      security-events: write\n      id-token: write\n    steps:\n      - uses: actions/checkout@sha\n        with:\n          persist-credentials: false\n      - uses: ossf/scorecard-action@2d1146689b8cda280b9bc96326124645441f03bc\n        with:\n          publish_results: true\n      - uses: github/codeql-action/upload-sarif@sha\n",
        CLI_SOURCE_PATH: 'Use:   "discover",\nUse:   "inject-fault",\n',
    }
    for relative, content in files.items():
        write_fixture(relative, content)


def has_error(fragment: str) -> bool:
    return any(fragment in error for error in validate_root(FIXTURE_ROOT))


def self_test() -> list[str]:
    try:
        reset_fixture()
        if errors := validate_root(FIXTURE_ROOT):
            return [f"valid fixture failed: {error}" for error in errors]

        security = FIXTURE_ROOT / SECURITY_PATH
        security.write_text(
            security.read_text(encoding="utf-8").replace("v0.2.x", "v0.1.x"),
            encoding="utf-8",
        )
        if not has_error("current support row"):
            return ["stale support fixture unexpectedly passed"]

        reset_fixture()
        write_fixture(README_PATH, "[Missing](docs/missing.md)\n")
        if not has_error("missing relative link"):
            return ["broken-link fixture unexpectedly passed"]

        reset_fixture()
        write_fixture(README_PATH, "task missing:task\n")
        if not has_error("referenced task does not exist"):
            return ["missing-task fixture unexpectedly passed"]

        reset_fixture()
        write_fixture(README_PATH, "scripts/missing.sh\n")
        if not has_error("repository path does not exist"):
            return ["missing-path fixture unexpectedly passed"]

        reset_fixture()
        write_fixture(DEMO_PATH, "```bash\ndraforge missing-command\n```\n")
        if not has_error("CLI command does not exist"):
            return ["invalid-CLI fixture unexpectedly passed"]

        reset_fixture()
        values = FIXTURE_ROOT / VALUES_PATH
        values.write_text(
            values.read_text(encoding="utf-8").replace("port: 8080", "port: 9090"),
            encoding="utf-8",
        )
        if not has_error("Service port must remain 8080"):
            return ["wrong-Service-port fixture unexpectedly passed"]

        reset_fixture()
        write_fixture(DOKS_WORKFLOW_PATH, "on:\n  workflow_dispatch:\n  schedule:\n")
        if not has_error("manual-only"):
            return ["scheduled DOKS fixture unexpectedly passed"]

        reset_fixture()
        readme = FIXTURE_ROOT / README_PATH
        readme.write_text(
            readme.read_text(encoding="utf-8").replace(
                "api.scorecard.dev", "api.securityscorecards.dev"
            ),
            encoding="utf-8",
        )
        if not has_error("legacy OpenSSF Scorecard domain"):
            return ["legacy Scorecard domain fixture unexpectedly passed"]

        reset_fixture()
        governance = FIXTURE_ROOT / GOVERNANCE_PATH
        governance.write_text(
            governance.read_text(encoding="utf-8")
            + "The project recognizes two valid ways. A second qualified human may be appointed.\n",
            encoding="utf-8",
        )
        if not has_error("second-maintainer continuity path is not allowed"):
            return ["second-maintainer continuity fixture unexpectedly passed"]
        return []
    finally:
        shutil.rmtree(FIXTURE_ROOT, ignore_errors=True)


def main() -> int:
    allowed = {"--self-test"}
    arguments = set(sys.argv[1:])
    unknown = arguments - allowed
    if unknown:
        print(f"unsupported arguments: {sorted(unknown)}", file=sys.stderr)
        return 2

    errors: list[str] = []
    if "--self-test" in arguments:
        fixture_errors = self_test()
        if fixture_errors:
            errors.extend(fixture_errors)
        else:
            print("==> Documentation verifier fixtures passed.")
    errors.extend(validate_root(ROOT))

    if errors:
        print("Documentation contract failed:", file=sys.stderr)
        for error in errors:
            print(f"  - {error}", file=sys.stderr)
        return 1
    print("==> Documentation contract passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
