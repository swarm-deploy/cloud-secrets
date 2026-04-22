# Использование с HashiCorp Vault (KV v2)

## Требования
- Развёрнутый Vault, доступный с manager-нод Docker Swarm.
- Включённый `KV v2` mount (например, `secret/`).
- Vault token с правами `list` + `read` для `metadata` и `data` путей выбранного префикса.
- Для локального примера ниже используется **Vault dev mode** (только для тестов).

## Как маппятся ключи Vault в Swarm
- Каждый key внутри одного Vault секрета (`data`) синхронизируется как отдельный Docker Swarm secret.
- Это правило действует всегда, даже если ключ только один.
- Формат имени external path: `<vault-path>/<key>`.
- Пример: если в `secret/cloud-secrets/users-db` лежат ключи `username` и `password`, то в Swarm появятся:
  - `cloud-secrets/users-db/username`
  - `cloud-secrets/users-db/password`
- Если в секрете только один ключ `value`, то путь в Swarm будет `cloud-secrets/users-db/value`.

Пример policy для префикса `cloud-secrets/` в mount `secret`:

```hcl
path "secret/metadata/cloud-secrets/*" {
  capabilities = ["read", "list"]
}

path "secret/data/cloud-secrets/*" {
  capabilities = ["read"]
}
```

## 1) Полный docker-compose с Vault

Ниже полный `docker-compose.yaml` для локального сценария:
- поднимает `vault` (dev mode),
- инициализирует `kv-v2` mount и тестовый секрет (`vault-init`),
- запускает `cloud-secrets` в режиме `CS_PROVIDER=vault`.

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

## 2) Создать Docker Swarm secret для токена из compose примера

```sh
echo "root-token" > vault_token
docker secret create vault_token ./vault_token
```

## 3) Задеплоить стек

```sh
docker stack deploy -c vault-compose.yaml cloud-secrets --detach=false
```

## 4) Проверить синхронизацию

1. Проверить первый запуск в логах сервиса (`secrets_created`, `secrets_updated`, `secrets_skipped`).
2. Проверить ручной trigger через `SIGHUP`:

```sh
docker ps --filter "name=cloud-secrets_cloud-secrets" --format "{{.ID}}" | xargs -r docker kill --signal=SIGHUP
```

3. Проверить метрики:
   - `cloud_secrets_provider_requests_total`
   - `cloud_secrets_provider_request_duration_seconds`
