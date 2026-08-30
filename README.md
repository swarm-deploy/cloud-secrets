# cloud-secrets

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/swarm-deploy/cloud-secrets/badge)](https://scorecard.dev/viewer/?uri=github.com/swarm-deploy/cloud-secrets)

**cloud-secrets** is a background service that synchronizes secrets from external secret managers with Docker Swarm.

It automatically detects secret changes, updates Docker Swarm secrets, and rolls out the affected services - without requiring changes to stack YAML files.

Supported cloud providers:
- [Cloud.ru Secret Manager](./docs/usage_cloudru.md)
- [HashiCorp Vault (KV v2)](./docs/usage_vault.md)

## How it works

```mermaid
flowchart TD
    A[cloud-secrets starts] --> B[Load config from env vars]
    B --> C["Create Docker Swarm<br/>and Cloud clients"]
    C --> E[Application sync loop]

    F[Trigger by timer] --> E
    Q[Trigger by SIGHUP] --> E

    E --> G[Read secrets from Cloud]
    E --> H[Read secrets from Swarm]
    G --> I["Compare by logical path<br/>and external version id"]
    H --> I

    I --> J{Secret state in Swarm}
    J -->|not exists| K[Create new Swarm secret]
    J -->|version changed| L[Create new secret version]
    J -->|same version| M[Skip]

    L --> N["Update services to use new secret ID"]
    N --> R[Rolls updated service tasks]
    R --> O[Remove old versions]
    O --> S[Restore parent secret]
    S --> T["Reload Swarm state"]
    T --> U{"CS_CLEANUP_ORPHANED=true"}
    U -->|yes| V["Remove managed secrets absent in Cloud<br/>and unused by services, with all versions"]
    U -->|no| P

    K --> P[Write sync result logs]
    V --> P
    M --> P
```

## Design goals

- External secret manager is the source of truth
- Stack files remain static
- Secret rotation requires no manual Docker operations
- Secrets are never stored in Git
- Runs natively inside Docker Swarm

## Monitoring

- [Grafana dashboard](grafana-dashboard.json)
