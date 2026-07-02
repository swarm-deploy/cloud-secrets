# cloud-secrets

**cloud-secrets** - background service for update secrets in Docker Swarm cluster

Supported cloud providers:
- [Cloud.ru Secret Manager](./docs/usage_cloudru.md)

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

## Monitoring
- [Grafana dashboard](grafana-dashboard.json)
