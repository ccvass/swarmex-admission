<p align="center"><img src="https://raw.githubusercontent.com/ccvass/swarmex/main/docs/assets/logo.svg" alt="Swarmex" width="400"></p>

[![Test, Build & Deploy](https://github.com/ccvass/swarmex-admission/actions/workflows/publish.yml/badge.svg)](https://github.com/ccvass/swarmex-admission/actions/workflows/publish.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

# Swarmex Admission

Admission controller that validates and mutates Docker Swarm services with namespace resource quotas.

Part of [Swarmex](https://github.com/ccvass/swarmex) — enterprise-grade orchestration for Docker Swarm.

## What It Does

Intercepts service creation and enforces policies: requires memory limits, requires team labels, auto-adds managed-by metadata. Also enforces per-namespace resource quotas (max memory, max services) to prevent resource exhaustion.

## Labels

No service labels required. Policies and quotas are defined in the controller's configuration:

```yaml
# Validation rules
rules:
  - require-memory-limit    # Deny services without memory limits
  - require-team-label      # Deny services without team label
  - add-managed-by          # Auto-add swarmex managed-by label

# Namespace quotas
quotas:
  frontend:
    max_memory: "4g"
    max_services: 3
```

## How It Works

1. Intercepts Docker service create/update events.
2. Validates the service spec against configured rules.
3. Denies services that fail validation (no memory limit, no team label).
4. Mutates passing services by adding managed-by labels automatically.
5. Checks namespace quotas and denies if the namespace would exceed limits.

## Quick Start

```bash
docker service create \
  --name swarmex-admission \
  --mount type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock \
  --mount type=bind,src=/etc/swarmex/admission.yaml,dst=/config.yaml \
  ghcr.io/ccvass/swarmex-admission:latest
```

## Verified

Services denied without memory limit or team label. Labels auto-added on valid services. 4th service in a namespace correctly denied due to quota exceeded.

## License

Apache-2.0
