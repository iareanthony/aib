[![CI](https://github.com/matijazezelj/aib/actions/workflows/ci.yml/badge.svg)](https://github.com/matijazezelj/aib/actions/workflows/ci.yml)

# AIB — Assets in a Box

![AIB Web UI](assets/aib.png)

AIB helps you answer a practical question quickly: what depends on what in this environment?

Point it at your IaC, and it builds a graph of assets and dependencies in SQLite. Then you can inspect blast radius, drift, cert expiry, and security findings from one place.

It is part of the "in a box" toolkit alongside [SIB](https://github.com/matijazezelj/sib) and [NIB](https://github.com/matijazezelj/nib).

### Typical workflow

1. Scan one or more sources (`scan terraform`, `scan k8s`, ...)
2. Open the UI (`serve`) or query from CLI (`graph`, `impact`)
3. Run security checks (`graph audit`) and triage critical findings
4. Re-scan as infra changes to track drift

## Quick Start

Fastest path to value:

```bash
./bin/aib scan terraform /path/to/terraform.tfstate
./bin/aib scan k8s /path/to/manifests/
./bin/aib serve
```

Full setup options:

```bash
# Build from source (requires Go 1.25.7+)
git clone https://github.com/matijazezelj/aib.git && cd aib && make build

# Or install directly
go install github.com/matijazezelj/aib/cmd/aib@latest

# Or use Docker (includes optional Memgraph)
docker compose -f deploy/docker-compose.yml up --build

# Scan some infrastructure
./bin/aib scan terraform /path/to/terraform.tfstate
./bin/aib scan k8s /path/to/manifests/

# Start the web UI
./bin/aib serve   # http://localhost:8080
```

## Scanning Sources

AIB currently supports seven parsers. You can pass multiple paths, and cross-file references are resolved when possible.

Use the parser sections below as reference. Most teams start with Terraform + Kubernetes, then add other sources.

### Terraform State

Parses `.tfstate` files (100+ mapped resource types across AWS/GCP/Azure/Cloudflare/TLS). Edges come from `dependencies` and attribute references (`vpc_id`, `subnet_id`, etc.).

It also extracts security metadata such as `encrypted`, `storage_encrypted`, `publicly_accessible`, `deletion_protection`, `multi_az`, SG ingress/egress CIDRs, and S3 versioning/logging status.

Node IDs: `tf:<assetType>:<name>`.

```bash
aib scan terraform terraform.tfstate
aib scan terraform /path/to/terraform/directory/
aib scan terraform networking.tfstate compute.tfstate   # cross-state edges

# Remote backends (requires terraform CLI)
aib scan terraform --remote project/
aib scan terraform --remote --workspace='*' project-a/ project-b/
```

### Terraform Plan

Parses `terraform show -json` output for pre-deploy impact analysis. Changes are classified as create/update/delete/replace, and destructive actions can be scored for blast radius.

Node IDs are compatible with state-scanned nodes.

```bash
terraform plan -out=tfplan && terraform show -json tfplan > plan.json
aib scan terraform-plan plan.json
aib scan terraform-plan infra-plan.json services-plan.json
```

### Kubernetes / Helm

Scans YAML manifests or Helm charts and discovers workloads, services, ingresses, secrets, configmaps, and their relationships (selectors, TLS, mounts, `envFrom`, etc.).

Security context metadata is extracted per container (`privileged`, `runAsNonRoot`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation`, `runAsUser`) plus pod-level flags (`hostNetwork`, `hostPID`, `hostIPC`, `serviceAccountName`).

It also infers `connects_to` edges from runtime config values (for example service hosts in env vars and ConfigMaps), so app-to-service dependencies show up even when no explicit IaC edge exists.

Node IDs: `k8s:<assetType>:<namespace>/<name>`.

```bash
aib scan k8s deployment.yaml
aib scan k8s /path/to/manifests/
aib scan k8s /path/to/chart --helm --values=values-prod.yaml

# Live cluster scanning (requires kubectl)
aib scan k8s --live
aib scan k8s --live --kubeconfig=~/.kube/config --context=prod --namespace=app
```

### Ansible

Parses inventory files (INI/YAML) to discover hosts. With `--playbooks`, it can also discover containers and services from `docker_container` and `service` tasks.

Inventory variables are used to infer dependency edges (for example `db_host`, `redis_host`, `k8s_service`). When possible, inferred nodes include `connection_string` metadata so app-to-data paths are explicit in CLI and UI.

Node IDs: `ansible:<assetType>:<hostname>`.

```bash
aib scan ansible inventory.ini
aib scan ansible staging.ini production.ini --playbooks=./playbooks/
```

### Docker Compose

Parses Docker Compose files into services, networks, and volumes, then builds dependency edges (`depends_on`, network membership, volume mounts).

Edges keep connection evidence metadata (`via`, `raw_value`) so you can see why a relationship exists.

Node IDs: `compose:<assetType>:<name>`.

```bash
aib scan compose docker-compose.yml
aib scan compose docker-compose.yml docker-compose.override.yml
```

### CloudFormation

Parses AWS CloudFormation templates (YAML/JSON, ~40 mapped resource types). Edges come from `DependsOn`, `Ref`, `Fn::GetAtt`, and common property references (`VpcId`, `SubnetId`, `SecurityGroupIds`).

Each edge includes provenance metadata (`via`, `raw_value`). Security metadata includes `PubliclyAccessible`, `StorageEncrypted`, `DeletionProtection`, `MultiAZ`, SG ingress CIDRs, and S3 `AccessControl`.

Node IDs: `cfn:<assetType>:<logicalId>`.

```bash
aib scan cloudformation template.yaml
aib scan cloudformation vpc.yaml compute.yaml database.json
```

### Pulumi

Parses `pulumi stack export` JSON (~80 mapped resource types across AWS/GCP/Azure/Kubernetes/TLS). Edges come from dependency arrays, attribute references, and parent URNs.

Attribute edges keep provenance metadata (`via`, `raw_value`). Security metadata includes `encrypted`, `storageEncrypted`, `publiclyAccessible`, `deletionProtection`, `multiAz`, and ingress CIDRs.

Node IDs: `plm:<assetType>:<name>`.

```bash
aib scan pulumi stack-export.json
aib scan pulumi infra-stack.json app-stack.json
```

## Graph Queries

For daily use, these are the commands most people rely on:

- `aib graph show` for a quick snapshot
- `aib impact node <id>` when investigating risk
- `aib graph path <from> <to>` to verify dependency paths
- `aib graph audit` for security checks

```bash
aib graph show                             # summary (node/edge counts by type)
aib graph nodes                            # list all nodes
aib graph nodes --type=vm --source=terraform --provider=google
aib graph edges                            # list all edges
aib graph edges --type=depends_on --from=tf:vm:web-prod-1
aib graph neighbors tf:vm:web-prod-1       # direct neighbors
aib graph path <from-id> <to-id>           # shortest path
aib graph deps <node-id> --depth=10        # downstream dependencies
aib graph export --format=json             # also: dot, mermaid
aib graph prune --stale-days=30            # remove stale nodes
```

### JSON Output

All query commands support `--output json` (or `-o json`) so they can be piped into `jq`, scripts, and CI:

```bash
aib -o json graph nodes --type=vm | jq '.[].id'
aib -o json graph edges --type=connects_to | jq 'length'
aib -o json graph show | jq '.total_nodes'
aib -o json impact node tf:vm:web-prod-1 | jq '.blast_radius'
aib -o json graph spof | jq '.[0].node.id'
aib -o json certs list | jq '.[] | select(.status == "critical")'
aib -o json db stats | jq '{nodes: .total_nodes, edges: .total_edges}'
```

Supported commands: `graph show`, `graph nodes`, `graph edges`, `graph neighbors`, `graph path`, `graph deps`, `graph cycles`, `graph spof`, `graph orphans`, `graph audit`, `impact node`, `certs list`, `certs expiring`, `certs probe`, `db stats`, `version`.

## Analysis

These commands are most useful after data has already been scanned.

### Blast Radius

```
$ aib impact node tf:network:prod-vpc

Impact Analysis: tf:network:prod-vpc
   Type: network | Provider: google | Source: terraform

   Blast Radius: 4 affected assets

   tf:network:prod-vpc (network)
   ├── [connects_to] tf:subnet:prod-subnet (subnet)
   │   └── [connects_to] tf:vm:web-prod-1 (vm)
   │       └── [depends_on] tf:dns_record:web.example.com (dns_record)
   └── [depends_on] tf:database:cloudsql-prod (database)
```

### Security Audit

AIB includes a built-in security audit over the graph model. It runs 12 checks with three severities (`critical`, `warning`, `info`):

| Check | Severity | Description |
|-------|----------|-------------|
| `public-database` | critical | Databases with `publicly_accessible = true` |
| `unencrypted-storage` | critical | Databases/buckets without encryption at rest |
| `permissive-firewall` | critical | Security groups allowing ingress from `0.0.0.0/0` |
| `privileged-container` | critical | Kubernetes containers running in privileged mode |
| `host-namespace` | critical | Pods using `hostNetwork`, `hostPID`, or `hostIPC` |
| `no-deletion-protection` | warning | Databases without deletion protection |
| `single-az-database` | warning | Databases not configured for Multi-AZ |
| `public-bucket` | warning | S3 buckets with public ACLs |
| `public-load-balancer` | warning | LoadBalancer services exposed externally |
| `public-instance` | warning | VMs with public IP addresses |
| `orphan-secret` | info | Kubernetes secrets not referenced by any workload |
| `container-security-best-practices` | info | Containers missing `runAsNonRoot`, `readOnlyRootFilesystem`, or with `allowPrivilegeEscalation` |

```bash
aib graph audit                            # tabular report
aib -o json graph audit | jq '.findings[] | select(.severity == "critical")'
```

The web UI marks findings directly on nodes: **red border** for critical, **orange border** for warning. Clicking a flagged node opens the details panel with a findings banner.

### Cycles, SPOF, Orphans

```bash
aib graph cycles                           # circular dependencies
aib graph spof --min-affected=3 --limit=10 # single points of failure (by blast radius)
aib graph orphans                          # nodes with no edges
```

### Drift Detection

Every scan automatically compares results against the current database and reports changes. Drift is source-scoped (a Terraform scan won't flag Kubernetes nodes as removed).

```
$ aib scan cloudformation template_v2.yaml
Discovered 5 nodes, 5 edges
Drift: 1 added, 1 removed, 1 modified nodes; 1 added, 0 removed edges
```

Drift details are stored per scan: `GET /api/v1/scans/{id}/diff`.

### Certificates

```bash
aib certs probe example.com:443            # probe and track a TLS endpoint
aib certs list                             # all tracked certificates
aib certs expiring --days=30               # expiring within threshold
aib certs check                            # re-probe all known endpoints
```

When running `aib serve`, certificates are probed on a schedule (`certs.probe_interval`). Expiry alerts can be sent to stdout, a generic webhook (for example SIB), or Slack.

## Web UI & API

```bash
aib serve                                  # default :8080
aib serve --listen=:9090                   # custom port
aib serve --read-only                      # disable scan triggers
```

The UI is designed for investigation: find a node, inspect edges, and answer impact questions quickly.

Current UI features:

- resource-focused left sidebar with source grouping and quick source-only filtering
- declutter toggle and selectable layout modes (readable/balanced/compact)
- resizable left and right side panels
- improved base typography for readability
- connection-string-aware detail rendering with one-click `Copy` actions
- **Connection Evidence** section on edges (`via`, `raw_value`, `reference`) with All/connects_to filtering
- **Online Icons** toggle (consent-gated) for service/provider icons from [Simple Icons](https://simpleicons.org/)
- **Security Risk Indicators** with colored borders and findings banner in the detail panel
- Focus panel: Dependencies, Impact, Find Secrets Path, Find External Path

If the graph feels noisy, start with search + source filter, then use focus modes from a single node.

API docs are available at `/api/docs` (Swagger UI).

### External CLI Timeouts

For parser paths that call external tools (`kubectl`, `helm`, `terraform`), AIB applies a default command timeout when the caller does not provide a deadline.

### API Endpoints

The API mostly mirrors CLI capabilities. Most endpoints are read-only graph queries; `/api/v1/scan` is the main mutating endpoint.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Health check |
| `GET` | `/metrics` | Prometheus metrics (no auth) |
| `GET` | `/api/v1/graph` | Full graph (nodes + edges) |
| `GET` | `/api/v1/graph/nodes` | List nodes (`?type=`, `?source=`, `?provider=`) |
| `GET` | `/api/v1/graph/nodes/{id}` | Single node details |
| `GET` | `/api/v1/graph/edges` | List edges (`?type=`, `?from=`, `?to=`) |
| `GET` | `/api/v1/graph/shortest-path` | Shortest path (`?from=`, `?to=`) |
| `GET` | `/api/v1/graph/dependency-chain/{nodeId}` | Downstream dependencies (`?depth=`) |
| `GET` | `/api/v1/graph/analysis/cycles` | Circular dependencies |
| `GET` | `/api/v1/graph/analysis/spof` | Single points of failure (`?min_affected=`, `?limit=`) |
| `GET` | `/api/v1/graph/analysis/orphans` | Orphan nodes |
| `GET` | `/api/v1/graph/analysis/audit` | Security audit findings |
| `GET` | `/api/v1/impact/{nodeId}` | Blast radius |
| `GET` | `/api/v1/plan/impact` | Terraform plan impact analysis |
| `GET` | `/api/v1/certs` | All tracked certificates |
| `GET` | `/api/v1/certs/expiring` | Expiring certs (`?days=30`) |
| `GET` | `/api/v1/stats` | Summary statistics |
| `GET` | `/api/v1/scans` | Scan history |
| `GET` | `/api/v1/scans/{id}/diff` | Drift summary for a scan |
| `GET` | `/api/v1/scan/status` | Check if a scan is running |
| `POST` | `/api/v1/scan` | Trigger a scan (JSON body) |
| `GET` | `/api/v1/export/json` | Export graph as JSON |
| `GET` | `/api/v1/export/dot` | Export graph as Graphviz DOT |
| `GET` | `/api/v1/export/mermaid` | Export graph as Mermaid |
| `GET` | `/api/v1/openapi.json` | OpenAPI 3.0 spec |
| `GET` | `/api/docs` | Swagger UI |

### Triggering Scans via API

```bash
curl -X POST http://localhost:8080/api/v1/scan \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"source": "terraform", "paths": ["/opt/infra/terraform"]}'
```

Valid sources: `terraform`, `terraform-plan`, `kubernetes`, `kubernetes-live`, `ansible`, `compose`, `cloudformation`, `pulumi`, `all`.

### Authentication & Security

Protect API endpoints with bearer token auth:

```yaml
server:
  api_token: "${AIB_API_TOKEN}"   # or: AIB_SERVER_API_TOKEN=... aib serve
```

Auth applies to `/api/*` routes only. The web UI, static assets, `/healthz`, and `/metrics` are always accessible.

AIB is intended for trusted internal networks. It includes security headers (strict CSP), API rate limiting (10 req/s per IP), request body limits (1 MB), path traversal checks, and scan path allowlisting (`scan.allowed_paths`).

Do not expose it directly to the public internet; place it behind TLS/reverse proxy.

## Configuration

Defaults are usable as-is. For customization, create `aib.yaml` in the current directory or in `~/.aib/`:

```yaml
storage:
  path: "./data/aib.db"
  memgraph:
    enabled: false
    uri: "bolt://localhost:7687"

certs:
  probe_enabled: true
  probe_interval: "6h"
  alert_thresholds: [90, 60, 30, 14, 7, 1]

alerts:
  webhook:
    enabled: false
    url: "http://sib:8080/api/v1/events"
    headers:
      Authorization: "Bearer ${AIB_WEBHOOK_TOKEN}"
  stdout:
    enabled: true
  slack:
    enabled: false
    webhook_url: "https://hooks.slack.com/services/T.../B.../xxx"
    channel: ""

server:
  listen: ":8080"
  read_only: true
  api_token: "${AIB_API_TOKEN}"
  cors_origin: ""

scan:
  schedule: "4h"
  on_startup: true
  allowed_paths:
    - "/opt/infra/terraform"
    - "/opt/infra/k8s"
```

All settings support `${ENV_VAR}` expansion and can also be set with `AIB_`-prefixed environment variables (for example `AIB_STORAGE_PATH`, `AIB_SERVER_LISTEN`).

Logging flags: `--log-format` (`text`/`json`) and `--log-level` (`debug`/`info`/`warn`/`error`). Shell completion: `aib completion [bash|zsh|fish|powershell]`.

See [`configs/aib.yaml.example`](configs/aib.yaml.example) for the full reference.

For most deployments, these are the first settings worth overriding:

- `storage.path`
- `server.listen`
- `server.api_token`
- `scan.allowed_paths`

## Memgraph

AIB uses SQLite as the source of truth. You can optionally add [Memgraph](https://github.com/memgraph/memgraph) for faster blast-radius, shortest-path, and neighbor queries. If Memgraph is unavailable, AIB falls back to the local BFS engine.

Use Memgraph when graph size or query volume grows. For small environments, SQLite-only mode is usually enough.

```bash
# Start Memgraph
docker run -p 7687:7687 memgraph/memgraph-mage

# Enable in config
# storage.memgraph.enabled: true, uri: "bolt://localhost:7687"

# Sync existing data
aib graph sync
```

When enabled, writes go to SQLite and Memgraph through `SyncedStore`. You can rebuild Memgraph state any time with `aib graph sync`. The `deploy/docker-compose.yml` includes both services.

## Known Limitations

- **Single-instance**: no clustering or replication — suitable for up to ~10K assets
- **No built-in TLS**: `aib serve` binds plain HTTP; use a reverse proxy for HTTPS in production
- **Single API token**: no per-user RBAC; one token grants full API access
- **Parser coverage is partial**: Terraform 100+ types, Pulumi ~80, CloudFormation ~40 — not all provider resources are mapped
- **Designed for internal networks**: do not expose directly to the public internet without a reverse proxy and TLS

## Development

```bash
make build       # Build binary
make test        # Run tests
make fmt         # Format code
make lint        # Run linter
make clean       # Remove build artifacts
```

## License

Apache 2.0
