[![CI](https://github.com/matijazezelj/aib/actions/workflows/ci.yml/badge.svg)](https://github.com/matijazezelj/aib/actions/workflows/ci.yml)

# AIB — Assets in a Box

![AIB Web UI](assets/aib.png)

Lightweight, self-hosted infrastructure asset discovery and dependency mapping tool. Parses IaC sources (Terraform, Pulumi, CloudFormation, Kubernetes/Helm, Ansible, Docker Compose), builds a unified dependency graph, tracks certificate expiry, and provides blast radius analysis — "what breaks if X fails?"

Part of the "in a box" security toolbox alongside [SIB](https://github.com/matijazezelj/sib) (SIEM in a Box) and [NIB](https://github.com/matijazezelj/nib) (NIDS in a Box).

## Quick Start

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

AIB supports 7 IaC parsers. All support multiple paths with automatic cross-file edge resolution.

### Terraform State

Parses `.tfstate` files (100+ resource type mappings across AWS, GCP, Azure, Cloudflare, TLS). Edges come from `dependencies` arrays and attribute references (vpc_id, subnet_id, etc.). Dependency edges include `source=tfstate_dependency` and `reference` metadata; attribute edges include `via` and `raw_value`. Node IDs: `tf:<assetType>:<name>`.

```bash
aib scan terraform terraform.tfstate
aib scan terraform /path/to/terraform/directory/
aib scan terraform networking.tfstate compute.tfstate   # cross-state edges

# Remote backends (requires terraform CLI)
aib scan terraform --remote project/
aib scan terraform --remote --workspace='*' project-a/ project-b/
```

### Terraform Plan

Parses `terraform show -json` output for pre-deploy impact analysis. Classifies changes as create/update/delete/replace and computes blast radius for destructive actions. Node IDs are compatible with state-scanned nodes.

```bash
terraform plan -out=tfplan && terraform show -json tfplan > plan.json
aib scan terraform-plan plan.json
aib scan terraform-plan infra-plan.json services-plan.json
```

### Kubernetes / Helm

Scans YAML manifests or Helm charts. Discovers workloads, services, ingresses, secrets, configmaps and their relationships (label selector matching, TLS termination, volume mounts, envFrom refs).

Also infers workload interconnectivity (`connects_to`) from application config values (for example service URLs/hosts found in env vars and ConfigMap values), so app-to-service dependencies are visible even when they are expressed only as runtime connection strings. Node IDs: `k8s:<assetType>:<namespace>/<name>`.

```bash
aib scan k8s deployment.yaml
aib scan k8s /path/to/manifests/
aib scan k8s /path/to/chart --helm --values=values-prod.yaml

# Live cluster scanning (requires kubectl)
aib scan k8s --live
aib scan k8s --live --kubeconfig=~/.kube/config --context=prod --namespace=app
```

### Ansible

Parses inventory files (INI/YAML) to discover hosts. With `--playbooks`, also discovers containers and services from `docker_container` and `service` tasks.

Inventory var inference also builds interconnectivity edges between hosts and dependent services/databases (for example `db_host`, `redis_host`, `k8s_service`). When possible, inferred database/service nodes include `connection_string` metadata (e.g. PostgreSQL/Redis) so app-to-data paths are explicit in both CLI and UI.

Node IDs: `ansible:<assetType>:<hostname>`.

```bash
aib scan ansible inventory.ini
aib scan ansible staging.ini production.ini --playbooks=./playbooks/
```

### Docker Compose

Parses Docker Compose files to discover services, networks, and volumes with their dependency relationships (depends_on, network membership, volume mounts). Edges carry connection evidence metadata (`via`, `raw_value`) so the UI and API show exactly how each relationship was discovered. Node IDs: `compose:<assetType>:<name>`.

```bash
aib scan compose docker-compose.yml
aib scan compose docker-compose.yml docker-compose.override.yml
```

### CloudFormation

Parses AWS CloudFormation templates (YAML/JSON, ~40 resource type mappings). Edges from DependsOn, Ref, Fn::GetAtt, and property references (VpcId, SubnetId, SecurityGroupIds). Each edge includes `via` and `raw_value` metadata distinguishing DependsOn vs Ref provenance. Node IDs: `cfn:<assetType>:<logicalId>`.

```bash
aib scan cloudformation template.yaml
aib scan cloudformation vpc.yaml compute.yaml database.json
```

### Pulumi

Parses `pulumi stack export` JSON output (~80 resource type mappings across AWS, GCP, Azure, Kubernetes, TLS). Edges from dependency arrays, attribute references, and parent URNs. Attribute edges carry `via` and `raw_value` metadata (e.g. `via=vpcId`, `raw_value=vpc-abc123`). Node IDs: `plm:<assetType>:<name>`.

```bash
aib scan pulumi stack-export.json
aib scan pulumi infra-stack.json app-stack.json
```

## Graph Queries

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

All query commands support `--output json` (or `-o json`) for machine-readable output, making it easy to pipe into `jq`, scripts, or CI pipelines:

```bash
aib -o json graph nodes --type=vm | jq '.[].id'
aib -o json graph edges --type=connects_to | jq 'length'
aib -o json graph show | jq '.total_nodes'
aib -o json impact node tf:vm:web-prod-1 | jq '.blast_radius'
aib -o json graph spof | jq '.[0].node.id'
aib -o json certs list | jq '.[] | select(.status == "critical")'
aib -o json db stats | jq '{nodes: .total_nodes, edges: .total_edges}'
```

Supported commands: `graph show`, `graph nodes`, `graph edges`, `graph neighbors`, `graph path`, `graph deps`, `graph cycles`, `graph spof`, `graph orphans`, `impact node`, `certs list`, `certs expiring`, `certs probe`, `db stats`, `version`.

## Analysis

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

When running `aib serve`, certificates are probed automatically on a schedule (configurable via `certs.probe_interval`). Expiry alerts can be sent to stdout, a generic webhook (e.g. SIB), or Slack (Block Kit formatted messages with severity colors and blast radius details).

## Web UI & API

```bash
aib serve                                  # default :8080
aib serve --listen=:9090                   # custom port
aib serve --read-only                      # disable scan triggers
```

The web UI provides interactive graph visualization with search, type/source filtering, blast radius highlighting, and focus modes.

Recent UI capabilities include:

- resource-focused left sidebar with source grouping and quick source-only filtering
- declutter toggle and selectable layout modes (readable/balanced/compact)
- resizable left and right side panels
- larger typography for improved readability
- connection-string-aware detail rendering with one-click `Copy` actions
- **Connection Evidence** section on edges showing how each relationship was discovered (`via`, `raw_value`, `reference`) with All/connects_to filter
- **Online Icons** toggle (consent-gated) that fetches service/provider icons from [Simple Icons](https://simpleicons.org/) CDN and overlays them on graph nodes (Kubernetes, Terraform, Docker, Redis, PostgreSQL, Ansible, etc.)
- Graph Focus panel with Dependencies, Impact, Find Secrets Path, and Find External Path modes

API docs are available at `/api/docs` (Swagger UI).

### External CLI Timeouts

For parser paths that call external tools (`kubectl`, `helm`, `terraform`), AIB applies a default command timeout when no deadline is provided by the caller. This improves robustness in CI and prevents long-running scans from hanging indefinitely.

### API Endpoints

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

AIB is designed for trusted internal networks. The server includes security headers (CSP with strict `default-src 'self'`, allowlisted CDN origins for icons), rate limiting (10 req/s per IP), request body limits (1 MB), path traversal protection, and a scan path allowlist (`scan.allowed_paths`). Do not expose to the public internet without a reverse proxy and TLS.

## Configuration

AIB works out of the box with sensible defaults. For customization, create `aib.yaml` in the current directory or `~/.aib/`:

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

All settings support `${ENV_VAR}` expansion and can be set via environment variables with the `AIB_` prefix (e.g. `AIB_STORAGE_PATH`, `AIB_SERVER_LISTEN`). Logging is controlled via `--log-format` (text/json) and `--log-level` (debug/info/warn/error) flags. Shell completions: `aib completion [bash|zsh|fish|powershell]`.

See [`configs/aib.yaml.example`](configs/aib.yaml.example) for the full reference.

## Memgraph

AIB uses SQLite as the persistent source of truth. Optionally, [Memgraph](https://github.com/memgraph/memgraph) can be added as a graph traversal engine for faster blast radius, shortest path, and neighbor queries. If Memgraph is unavailable, all queries fall back to the local BFS engine transparently.

```bash
# Start Memgraph
docker run -p 7687:7687 memgraph/memgraph-mage

# Enable in config
# storage.memgraph.enabled: true, uri: "bolt://localhost:7687"

# Sync existing data
aib graph sync
```

Writes go to both SQLite and Memgraph via the `SyncedStore` decorator. Memgraph can be rebuilt at any time with `aib graph sync`. The `deploy/docker-compose.yml` includes both AIB and Memgraph pre-configured.

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
