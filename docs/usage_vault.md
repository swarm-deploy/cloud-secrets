# Using HashiCorp Vault (KV v2)

**Requirements**
- A running Vault instance reachable from Docker Swarm manager nodes.
- An enabled `KV v2` mount (for example, `secret/`).
- A Vault token with `list` and `read` permissions for `metadata` and `data` paths under your selected prefix.

## How Vault Keys Map to Swarm Secrets
- Each key inside one Vault secret (`data`) is synchronized as a separate Docker Swarm secret.
- This rule always applies, even when a Vault secret has only one key.
- External path format: `<vault-path>/<key>`.
- Example: if `secret/cloud-secrets/users-db` contains `username` and `password`, Swarm secrets will be:
  - `cloud-secrets-users-db-username`
  - `cloud-secrets-users-db-password`

Example policy for the `cloud-secrets/` prefix in the `secret` mount:

```hcl
path "secret/metadata/cloud-secrets/*" {
  capabilities = ["read", "list"]
}

path "secret/data/cloud-secrets/*" {
  capabilities = ["read"]
}
```

## Deploy

Below is a full local `docker-compose.yaml` example that:
- starts `vault` in dev mode,
- initializes the `kv-v2` mount and a test secret via `vault-init`,
- starts `cloud-secrets` with `CS_PROVIDER=vault`.

<details>
  <summary>docker-compose.yaml</summary>

```yaml
version: '3.8'

services:
  vault:
    image: hashicorp/vault:1.18
    ports:
      - "8200:8200"
    cap_add:
      - IPC_LOCK
    command: vault server -dev -dev-root-token-id=root-token -dev-listen-address=0.0.0.0:8200
    deploy:
      placement:
        constraints:
          - node.role == manager

  vault-init:
    image: hashicorp/vault:1.18
    environment:
      - VAULT_ADDR=http://vault:8200
      - VAULT_TOKEN=root-token
    command: >
      sh -ec "
        until vault status >/dev/null 2>&1; do sleep 1; done;
        vault secrets enable -path=secret kv-v2 || true;
        vault kv put secret/cloud-secrets/users-service-db-dsn value='postgres://user:pass@db:5432/app';
      "
    deploy:
      restart_policy:
        condition: none
      placement:
        constraints:
          - node.role == manager

  cloud-secrets:
    image: swarmdeployorg/cloud-secrets:v0.1.0
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
    environment:
      - CS_PROVIDER=vault
      - CS_REFRESH_INTERVAL=10s
      - CS_LOG_LEVEL=debug
      - VAULT_ADDRESS=http://vault:8200
      - VAULT_TOKEN=/var/run/secrets/vault_token
      - VAULT_MOUNT_PATH=secret
      - VAULT_PREFIX=cloud-secrets
    secrets:
      - vault_token
    deploy:
      labels:
        - prometheus.port=8000
      placement:
        constraints:
          - node.role == manager

secrets:
  vault_token:
    external: true
```
</details>

&raquo; &nbsp;1. Copy docker-compose.yaml

<details>
  <summary>2. Create a Docker Swarm Secret for the Vault Token</summary>

```sh
printf %s "root-token" > vault_token
docker secret create vault_token ./vault_token
```
</details>

<details>
  <summary>3. Deploy the Stack</summary>

```sh
docker stack deploy -c vault-compose.yaml cloud-secrets --detach=false
```
</details>
