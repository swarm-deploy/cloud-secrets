# cloud-secrets

**cloud-secrets** - background service for update secrets in Docker Swarm cluster

Supported cloud providers:
- [Cloud.ru](https://cloud.ru/docs/scsm/ug/index)

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

    K --> P[Write sync result logs]
    S --> P
    M --> P
```

## Monitoring
- [Grafana dashboard](grafana-dashboard.json)

## Usage

### Usage with Cloud.ru

#### Create IAM secrets
```
echo "<client-id>" > iam_id
echo "<client-secret>" > iam_secret

docker secret create iam_id ./iam_id
docker secret create iam_secret ./iam_secret
```

##### Deploy Docker stack

Run `docker stack deploy -c docker-compose.yaml cloud-secrets --detach=false`

docker-compose.yaml
```yaml
version: '3.8'

services:
  swarm-secrets:
    image: swarmdeployorg/cloud-secrets:v0.1.0
    ports:
      - "8000:8000"
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
    environment:
      - CS_REFRESH_INTERVAL=10s
      - CLOUDRU_PROJECT_ID=<uuid>
      - CLOUDRU_IAM_CLIENT_ID=/var/run/secrets/iam_id
      - CLOUDRU_IAM_CLIENT_SECRET=/var/run/secrets/iam_secret
    secrets:
      - iam_id
      - iam_secret
    deploy:
      labels:
        - prometheus.port=8000
      placement:
        constraints:
          - node.role == manager

secrets:
  iam_id:
    external: true
  iam_secret:
    external: true
```
