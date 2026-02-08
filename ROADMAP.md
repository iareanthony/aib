# AIB Roadmap

## Completed

### v0.1.0 — Core Foundation
- [x] SQLite persistent store with node/edge CRUD
- [x] Memgraph optional graph engine with automatic fallback
- [x] Terraform state parser (GCP, AWS, Azure, Cloudflare — 100+ resource types)
- [x] Kubernetes manifest parser (Deployments, Services, Ingresses, Secrets, ConfigMaps)
- [x] Helm chart support via `helm template`
- [x] Ansible inventory + playbook parser
- [x] Blast radius analysis (upstream BFS)
- [x] CLI: scan, graph show/nodes/edges/neighbors/path, impact
- [x] REST API with embedded Cytoscape.js web UI
- [x] Certificate tracking and TLS probing
- [x] Webhook + stdout alerting

### v0.2.0 — Graph Queries & Compose
- [x] Docker Compose parser (services, networks, volumes, polymorphic depends_on)
- [x] Graph export: JSON, DOT, Mermaid
- [x] Shortest path and dependency chain queries
- [x] Prometheus metrics endpoint (`/metrics`)
- [x] Database backup command
- [x] Cross-state edge resolution for multi-file Terraform scanning
- [x] Live Kubernetes cluster scanning via kubectl

### v0.3.0 — Hardening & DX
- [x] API authentication (bearer token)
- [x] Security headers, rate limiting, request body limits, path validation
- [x] CORS support
- [x] SyncedStore decorator (SQLite + Memgraph dual writes)
- [x] gosec / errcheck / golangci-lint v2 compliance
- [x] CI pipeline (test + lint + gosec) and release pipeline (linux/amd64 + darwin/arm64)

### v0.3.1 — Analysis & Plan Impact
- [x] Graph analysis: cycle detection, single points of failure, orphan nodes
- [x] Terraform plan parser (`terraform show -json` output)
- [x] Plan impact API with blast radius for destructive changes
- [x] Shell completion (bash/zsh/fish/powershell)
- [x] JSON structured logging (`--log-format`, `--log-level`)
- [x] Config validation with multi-error reporting
- [x] Comprehensive test coverage (graph 86%, server 63%, scanner 60%, terraform 66%)

---

## Planned

### #6 — CloudFormation Parser
Parse AWS CloudFormation templates (JSON/YAML) to discover resources and dependencies. Map CFN resource types (`AWS::EC2::Instance`, `AWS::RDS::DBInstance`, etc.) to existing asset types. Support nested stacks and cross-stack references.

**Priority**: High — completes the major IaC trifecta (Terraform, K8s, CloudFormation).

### #8 — OpenAPI Spec
Generate and serve OpenAPI/Swagger documentation at `/api/docs`. Auto-generate from existing handler signatures and route registrations.

**Priority**: Medium — improves API discoverability for integrations.

### #9 — Pulumi Support
Parse Pulumi state files to discover infrastructure. Similar to Terraform state parsing but with Pulumi's state format and resource naming conventions.

**Priority**: Medium — growing Pulumi adoption in the ecosystem.

### #10 — Diff / Drift Detection
Compare two scans over time to detect infrastructure drift. Show added/removed/changed nodes and edges between scan snapshots. Useful for detecting unauthorized changes.

**Priority**: Medium — high value for security posture monitoring.

### #11 — CLI Coverage Improvement
Refactor `cmd/aib/main.go` to support dependency injection for `openStore()` / `os.Exit()`, enabling unit testing of command handlers without integration overhead. Current coverage is 9.7%.

**Priority**: Low — mostly ceremony, existing integration tests cover the important paths.

### #12 — Enhanced Metadata Extraction
Extend `extractMetadata` to handle provider-specific field names:
- AWS RDS `instance_class` (currently only `instance_type` is extracted)
- Azure `sku` fields
- GCP `tier` / `edition` fields

**Priority**: Low — cosmetic improvement, doesn't affect graph structure.

### Stretch Goals
- **Graph diff visualization** in the web UI (side-by-side or overlay)
- **Slack/Teams alerting** backends (in addition to webhooks)
- **Policy engine** — define rules like "no orphan nodes in production" or "max blast radius < 10"
- **Multi-tenant support** — separate graphs per team/environment with RBAC
- **Terraform Cloud/Enterprise integration** — pull state directly from TFC API
