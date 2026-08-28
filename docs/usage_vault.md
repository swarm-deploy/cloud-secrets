# Using HashiCorp Vault (KV v2)

[На Русском](./usage_vault_ru.md)

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
  <summary>5. Copy docker-compose.yaml for cloud-secrets Stack</summary>

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

## Setup with Vault AppRole

**Requirements**

* A running Vault instance reachable from Docker Swarm manager nodes.
  If `VAULT_ADDR` uses the service name `vault`, run Vault on a Swarm overlay network that is also attached to the `cloud-secrets` stack.
* An enabled `KV v2` mount (for example, `prod/`).
* An enabled Vault `AppRole` auth method.
* A Vault AppRole with a policy that has `list` and `read` permissions for the configured KV mount.

**Steps**

<details>
  <summary>1. Create new KV secrets engine in Vault</summary>

1. On the Secret Engine creation page, select KV.

![](./screenshots/vault_1_1_se_kv.png)

2. Enter the path `prod`.

![](./screenshots/vault_1_2_se_name.png)

3. Click the `Enable engine` button.

</details>

<details>
  <summary>2. Create ACL Policy</summary>

1. Enter the Name `cloud-secrets`.
2. Enter the Policy with definition:

```hcl
path "prod/metadata" {
  capabilities = ["list"]
}

path "prod/metadata/*" {
  capabilities = ["read", "list"]
}

path "prod/data/*" {
  capabilities = ["read"]
}
```

3. Click the `Create policy` button.

![](./screenshots/vault_3_1_acl.png)

</details>

<details>
  <summary>3. Enable AppRole authentication</summary>

Enable the AppRole auth method:

```sh
vault auth enable approle
```

If AppRole is already enabled, this step can be skipped.

</details>

<details>
  <summary>4. Create AppRole for cloud-secrets</summary>

Create an AppRole and attach the `cloud-secrets` policy:

```sh
vault write auth/approle/role/cloud-secrets \
  token_policies="cloud-secrets" \
  token_ttl="1h" \
  token_max_ttl="4h" \
  secret_id_ttl="0" \
  secret_id_num_uses="0"
```

`cloud-secrets` uses the RoleID and SecretID only to authenticate with Vault. The Vault runtime token returned by AppRole is kept in memory and is automatically obtained again when required.

In this example the SecretID does not expire and has no usage limit. Rotate the SecretID separately if required by your security policy.

</details>

<details>
  <summary>5. Get AppRole RoleID and SecretID</summary>

Get the RoleID:

```sh
vault read -field=role_id auth/approle/role/cloud-secrets/role-id
```

Generate a SecretID:

```sh
vault write -field=secret_id -f auth/approle/role/cloud-secrets/secret-id
```

Save both values. The SecretID is a secret and must not be committed to the repository.

</details>

<details>
  <summary>6. Create Docker Swarm Secrets for AppRole credentials</summary>

Create a Swarm secret containing the RoleID:

```sh
printf %s "<ROLE_ID>" | \
  docker secret create \
    --label cloud-secrets.secret.managed=true \
    vault-approle-role-id -
```

Create a Swarm secret containing the SecretID:

```sh
printf %s "<SECRET_ID>" | \
  docker secret create \
    --label cloud-secrets.secret.managed=true \
    vault-approle-secret-id -
```

The secrets will be mounted into the `cloud-secrets` container at:

```text
/run/secrets/vault-approle-role-id
/run/secrets/vault-approle-secret-id
```

</details>

<details>
  <summary>7. Copy docker-compose.yaml for cloud-secrets Stack </summary>

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
      - VAULT_MOUNT_PATH=prod
      - VAULT_AUTH_APPROLE_ROLE_ID=/run/secrets/vault-approle-role-id
      - VAULT_AUTH_APPROLE_SECRET_ID=/run/secrets/vault-approle-secret-id
    networks:
      - vault
    secrets:
      - vault-approle-role-id
      - vault-approle-secret-id
    deploy:
      labels:
        - prometheus.port=8000
      placement:
        constraints:
          - node.role == manager

networks:
  vault:
    external: true

secrets:
  vault-approle-role-id:
    external: true
  vault-approle-secret-id:
    external: true
```

The external `vault` network must be the same Swarm overlay network where the Vault service is reachable as `vault`.
</details>

<details>
  <summary>8. Deploy the Stack</summary>

```sh
docker stack deploy -c docker-compose.yaml cloud-secrets --detach=false
```
</details>
