# Using cloud-secrets with HashiCorp Vault (KV v2)

[На Русском](./usage_vault_ru.md)

How Vault keys are converted to Docker Swarm secrets:
- Each key inside one Vault secret (`data`) is synchronized as a separate Docker Swarm secret.
- This rule always applies, even when a Vault secret has only one key.
- The configured `VAULT_MOUNT_PATH` is not included in the Docker Swarm secret name.
- External path format: `<path-inside-mount>/<key>`.

> [!TIP]
> Example: if one Vault secret is created at `prod/orders` and has the keys `db-user` and `db-password`
>
> Then `prod` is the KV v2 mount path, `orders` is the path inside the mount, and `cloud-secrets` creates two secrets in Docker Swarm:
> * `orders-db-user`
> * `orders-db-password`

## Using cloud-secrets with Vault AppRole

**Prerequisites**

* A running Vault instance reachable from Docker Swarm manager nodes.
* In the examples below, we assume Vault is deployed in the `vault` network and is available at `vault:8200`.

In the instructions below, we will create a KV Engine, an AppRole, and secrets for **cloud-secrets** to work with Vault.

> [!IMPORTANT]
> cloud-secrets uses RoleID and SecretID only to authenticate with Vault.
>
> The Vault runtime token returned by AppRole is stored in memory and automatically requested again when needed.
>
> In the examples below, SecretID does not expire and has no usage limit. Rotate SecretID separately if required by your security policy.


### Configuring Vault with cloud-secrets CLI

The CLI performs the following actions:

&raquo; Enables AppRole authentication

<details>
  <summary>Creates an ACL Policy named cloud-secrets</summary>

```hcl
path "mountPath/metadata" {
  capabilities = ["list"]
}

path "mountPath/metadata/*" {
  capabilities = ["read", "list"]
}

path "mountPath/data/*" {
  capabilities = ["read"]
}
```
</details>

<details>
  <summary>Creates an AppRole named cloud-secrets</summary>

```
name=cloud-secrets
token_policies=cloud-secrets
token_ttl=1h
token_max_ttl=4h
secret_id_ttl=0
secret_id_num_uses=0
```

</details>

<details>
  <summary>Creates Swarm secrets for authenticating cloud-secrets requests to Vault</summary>

- cloud-secrets-vault-approle-role-id
- cloud-secrets-vault-approle-secret-id
</details>

> [!IMPORTANT]
> To configure AppRole, the CLI will ask for a Vault token with permissions to manage ACL policies and AppRole.
>
> The token is used only during setup and is not saved in Docker Swarm.


#### Steps

<details>
  <summary>1. Create a new KV secrets engine in Vault</summary>

1. On the Secret Engine creation page, select KV.

![](./screenshots/vault_1_1_se_kv.png)

2. Enter the path `prod`.

![](./screenshots/vault_1_2_se_name.png)

3. Click the `Enable engine` button.

</details>

<details>
  <summary>2. Run the cloud-secrets CLI</summary>

Run the following command:

```shell
docker run --rm --network=vault -it -v /var/run/docker.sock:/var/run/docker.sock:ro swarmdeployorg/cloud-secrets:v0.4.0 vault approle vault:8200 prod
```

Where:
- `vault:8200` is the Vault address.
- `prod` is the KV v2 mount path.

</details>

<details>
  <summary>3. Copy docker-compose.yaml for the cloud-secrets Stack</summary>

```yaml
version: '3.8'

services:
  cloud-secrets:
    image: swarmdeployorg/cloud-secrets:v0.4.0
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
    environment:
      - CS_PROVIDER=vault
      - CS_REFRESH_INTERVAL=1m
      - CS_LOG_LEVEL=info
      - VAULT_ADDR=http://vault:8200
      - VAULT_MOUNT_PATH=prod
      - VAULT_AUTH_APPROLE_ROLE_ID=/run/secrets/cloud-secrets-vault-approle-role-id
      - VAULT_AUTH_APPROLE_SECRET_ID=/run/secrets/cloud-secrets-vault-approle-secret-id
    networks:
      - vault
    secrets:
      - cloud-secrets-vault-approle-role-id
      - cloud-secrets-vault-approle-secret-id
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
  cloud-secrets-vault-approle-role-id:
    external: true
  cloud-secrets-vault-approle-secret-id:
    external: true
```

The external `vault` network must be the same Swarm overlay network where the Vault service is reachable as `vault`.
</details>

<details>
  <summary>4. Deploy the cloud-secrets stack</summary>

```sh
docker stack deploy -c docker-compose.yaml cloud-secrets --detach=false
```
</details>

### Fully Manual Vault Configuration

#### Steps

<details>
  <summary>1. Create a new KV secrets engine in Vault</summary>

1. On the Secret Engine creation page, select KV.

![](./screenshots/vault_1_1_se_kv.png)

2. Enter the path `prod`.

![](./screenshots/vault_1_2_se_name.png)

3. Click the `Enable engine` button.

</details>

<details>
  <summary>2. Create an ACL Policy</summary>

1. Enter the Name `cloud-secrets`.
2. Enter the Policy with the following contents:

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

Enable the AppRole authentication method:

```sh
vault auth enable approle
```

If AppRole is already enabled, this step can be skipped.

</details>

<details>
  <summary>4. Create an AppRole for cloud-secrets</summary>

Create an AppRole and attach the `cloud-secrets` policy to it:

```sh
vault write auth/approle/role/cloud-secrets \
  token_policies="cloud-secrets" \
  token_ttl="1h" \
  token_max_ttl="4h" \
  secret_id_ttl="0" \
  secret_id_num_uses="0"
```

</details>

<details>
  <summary>5. Get the AppRole RoleID and SecretID</summary>

Get the RoleID:

```sh
vault read -field=role_id auth/approle/role/cloud-secrets/role-id
```

Generate a SecretID:

```sh
vault write -field=secret_id -f auth/approle/role/cloud-secrets/secret-id
```

Save both values. SecretID is a secret and must not be committed to the repository.

</details>

<details>
  <summary>6. Create Docker Swarm Secrets for AppRole credentials</summary>

Create a Swarm secret with RoleID:

```sh
printf %s "<ROLE_ID>" | \
  docker secret create \
    cloud-secrets-vault-approle-role-id -
```

Create a Swarm secret with SecretID:

```sh
printf %s "<SECRET_ID>" | \
  docker secret create \
    cloud-secrets-vault-approle-secret-id -
```

The secrets will be mounted into the `cloud-secrets` container at:

```text
/run/secrets/cloud-secrets-vault-approle-role-id
/run/secrets/cloud-secrets-vault-approle-secret-id
```

</details>

<details>
  <summary>7. Copy docker-compose.yaml for the cloud-secrets Stack</summary>

```yaml
version: '3.8'

services:
  cloud-secrets:
    image: swarmdeployorg/cloud-secrets:v0.4.0
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
    environment:
      - CS_PROVIDER=vault
      - CS_REFRESH_INTERVAL=1m
      - CS_LOG_LEVEL=info
      - VAULT_ADDR=http://vault:8200
      - VAULT_MOUNT_PATH=prod
      - VAULT_AUTH_APPROLE_ROLE_ID=/run/secrets/cloud-secrets-vault-approle-role-id
      - VAULT_AUTH_APPROLE_SECRET_ID=/run/secrets/cloud-secrets-vault-approle-secret-id
    networks:
      - vault
    secrets:
      - cloud-secrets-vault-approle-role-id
      - cloud-secrets-vault-approle-secret-id
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
  cloud-secrets-vault-approle-role-id:
    external: true
  cloud-secrets-vault-approle-secret-id:
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

## Using cloud-secrets with Vault Token

**Prerequisites**

* A running Vault instance reachable from Docker Swarm manager nodes.
* In this example, we assume Vault is deployed in the `vault` network and is available at `vault:8200`.

### Steps

<details>
  <summary>1. Create a new KV secrets engine in Vault</summary>

1. On the Secret Engine creation page, select KV.

![](./screenshots/vault_1_1_se_kv.png)

2. Enter the path `prod`.

![](./screenshots/vault_1_1_se_kv.png)

3. Click the `Enable engine` button.
</details>

<details>
  <summary>2. Create an ACL Policy</summary>

1. Enter the Name `cloud-secrets`.
2. Enter the Policy with the following contents:

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

![](./screenshots/vault_1_1_se_kv.png)
</details>

<details>
  <summary>3. Create a new token for cloud-secrets</summary>

Inside the Vault container, create a token with the following command:

```sh
vault token create -policy=cloud-secrets
```
</details>

<details>
  <summary>4. Create a Docker Swarm Secret for the Vault Token</summary>

```sh
printf %s "<VAULT-TOKEN>" | \
  docker secret create \
    cloud-secrets-vault-auth-token -
```
</details>

<details>
  <summary>5. Copy docker-compose.yaml for the cloud-secrets Stack</summary>

```yaml
version: '3.8'

services:
  cloud-secrets:
    image: swarmdeployorg/cloud-secrets:v0.4.0
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
    environment:
      - CS_PROVIDER=vault
      - CS_REFRESH_INTERVAL=1m
      - CS_LOG_LEVEL=info
      - VAULT_ADDR=http://vault:8200
      - VAULT_AUTH_TOKEN=/var/run/secrets/cloud-secrets-vault-auth-token
      - VAULT_MOUNT_PATH=prod
    secrets:
      - cloud-secrets-vault-auth-token
    deploy:
      labels:
        - prometheus.port=8000
      placement:
        constraints:
          - node.role == manager

secrets:
  cloud-secrets-vault-auth-token:
    external: true
```

</details>

<details>
  <summary>6. Deploy the Stack</summary>

```sh
docker stack deploy -c docker-compose.yaml cloud-secrets --detach=false
```
</details>
