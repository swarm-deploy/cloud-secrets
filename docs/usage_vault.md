# Using HashiCorp Vault (KV v2)

How Vault Keys Map to Swarm Secrets:
- Each key inside one Vault secret (`data`) is synchronized as a separate Docker Swarm secret.
- This rule always applies, even when a Vault secret has only one key.
- External path format: `<vault-path>/<key>`.
- Example: if `secret/cloud-secrets/users-db` contains `username` and `password`, Swarm secrets will be:
  - `cloud-secrets-users-db-username`
  - `cloud-secrets-users-db-password`

## Manual setup with existing Vault installation

**Requirements**
- A running Vault instance reachable from Docker Swarm manager nodes.
- An enabled `KV v2` mount (for example, `secret/`).
- Either:
- A file-mounted Vault runtime token with `list` and `read` permissions.
- Or Vault AppRole credentials able to log in and obtain such a token.

<details>
  <summary>docker-compose.yaml</summary>

```yaml
version: '3.8'

services:
  cloud-secrets:
    image: swarmdeployorg/cloud-secrets:v0.4.0
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
    environment:
      - CS_PROVIDER=vault
      - CS_REFRESH_INTERVAL=10s
      - CS_LOG_LEVEL=debug
      - VAULT_ADDR=http://vault:8200
      - VAULT_AUTH_TOKEN=/var/run/secrets/vault-auth-token
      - VAULT_MOUNT_PATH=secret
      - VAULT_PREFIX=cloud-secrets
    secrets:
      - vault-auth-token
    deploy:
      labels:
        - prometheus.port=8000
      placement:
        constraints:
          - node.role == manager

secrets:
  vault-auth-token:
    external: true
```
</details>

Static token auth expects `VAULT_AUTH_TOKEN` to contain a path to a file with the Vault token, for example a mounted Docker secret.

AppRole auth uses:

```text
VAULT_AUTH_APPROLE_ROLE_ID
VAULT_AUTH_APPROLE_SECRET_ID
```

When both AppRole variables are set, `cloud-secrets` logs in via `POST /v1/auth/approle/login` and keeps the returned runtime token only in memory.

Steps:

<details>
  <summary>1. Create new KV secrets engine in Vault</summary>

1. On the Secret Engine creation page, select KV

![](./screenshots/vault_1_1_se_kv.png)

2. Enter the path `prod`

![](./screenshots/vault_1_1_se_kv.png)

3. Click the `Enable engine`  button
</details>

<details>
  <summary>2. Create ACL Policy</summary>

1. Enter the Name `cloud-secrets`
2. Enter the Policy with definition:
```hcl
path "prod/*" {
  capabilities = ["read", "list"]
}
```

3. Click the "Create policy" button

![](./screenshots/vault_1_1_se_kv.png)
</details>

<details>
  <summary>3. Create new token for cloud-secrets</summary>

Inside Vault container create token with follow command:

```sh
vault token create -policy=cloud-secrets
```
</details>

&raquo; &nbsp;4. Copy docker-compose.yaml

<details>
  <summary>2. Create a Docker Swarm Secret for the Vault Token</summary>

```sh
printf %s "root-token" > vault_auth_token
docker secret create vault-auth-token ./vault_auth_token
```
</details>

<details>
  <summary>5. Deploy the Stack</summary>

```sh
docker stack deploy -c docker-compose.yaml cloud-secrets --detach=false
```
</details>
