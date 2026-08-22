# Using HashiCorp Vault (KV v2)

How Vault Keys Map to Swarm Secrets:
- Each key inside one Vault secret (`data`) is synchronized as a separate Docker Swarm secret.
- This rule always applies, even when a Vault secret has only one key.
- External path format: `<vault-path>/<key>`.
- Example: if `secret/cloud-secrets/users-db` contains `username` and `password`, Swarm secrets will be:
  - `cloud-secrets-users-db-username`
  - `cloud-secrets-users-db-password`

## Setup with Vault Token

**Requirements**
- A running Vault instance reachable from Docker Swarm manager nodes.
- An enabled `KV v2` mount (for example, `secret/`).
- A file-mounted Vault runtime token with `list` and `read` permissions.

**Steps**

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

<details>
  <summary>4. Create a Docker Swarm Secret for the Vault Token</summary>

```sh
printf %s "root-token" > vault-auth-token
docker secret create vault-auth-token --label cloud-secrets.secret.managed=true ./vault-auth-token
```
</details>

<details>
  <summary>5. Copy docker-compose.yaml for cloud-secrets Stack </summary>

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
      - vault-auth-token=/var/run/secrets/vault-auth-token
      - VAULT_MOUNT_PATH=secret
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

<details>
  <summary>6. Deploy the Stack</summary>

```sh
docker stack deploy -c docker-compose.yaml cloud-secrets --detach=false
```
</details>
