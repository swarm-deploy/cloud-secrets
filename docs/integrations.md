# Integration API

`cloud-secrets` can expose an optional gRPC API for control-plane integrations such as `swarm-deploy`.
The API is disabled by default and does not add persistent state.

## Configuration

Set `CS_GRPC_ADDR` to the address that the gRPC server must bind to:

```text
CS_GRPC_ADDR=:8001
```

When the variable is unset or empty, the gRPC server is not started. The address only needs to be reachable on the shared Swarm overlay network; publishing the port is not required.

Prometheus metrics already listen on `:8000`. Configuring `CS_GRPC_ADDR=:8000` therefore causes startup to fail with an address-in-use error. `cloud-secrets` does not automatically move either endpoint when a configured address is occupied.

The presence of `CS_GRPC_ADDR` in the service environment can be used as an integration discovery signal together with the service image.

## Services

The protobuf package is `swarm_deploy.cloud_secrets`. The same gRPC listener exposes the existing `Controller` service and the `Secrets` service:

```proto
service Controller {
  rpc GetInfo(GetInfoRequest) returns (GetInfoResponse);
  rpc Sync(SyncRequest) returns (SyncResponse);
}

service Secrets {
  rpc Create(CreateSecretRequest) returns (CreateSecretResponse);
}

message CreateSecretRequest {
  string path = 1;
  bytes value = 2;
  string key = 3;
  string description = 4;
}

message CreateSecretResponse {
  string path = 1;
}
```

`GetInfo` returns the running version, configured provider name, documentation (`provider.links.doc`) and management UI (`provider.links.manager`) links when available, the timestamp of the last successful synchronization known by the current process, and the next scheduled interval synchronization time. `last_sync_at` is absent after process start until the first successful synchronization and is not persisted across restarts. `next_sync_at` follows the interval timer; manual gRPC and SIGHUP synchronizations do not reset it.

`Sync` invokes the same synchronization path used by interval and SIGHUP triggers. If another synchronization is already running, the RPC returns gRPC status `ABORTED`.

The response summarizes created, updated, removed, and unchanged logical secrets. Removed historical secret versions are not included in `removed`.

`Secrets.Create` creates a secret only in the configured external provider. It does not run synchronization and does not mutate Docker Swarm. `path` is relative to the provider's configured synchronization scope, `value` is passed as the secret payload, and the response contains only the accepted normalized path. Invalid provider-specific fields return `INVALID_ARGUMENT`; an existing secret or Vault key returns `ALREADY_EXISTS`.

Provider behavior differs:

- HashiCorp Vault KV v2 interprets `path` as the KV document path and requires `key` as the key inside that document. Creation preserves other keys and uses KV v2 check-and-set writes to avoid losing concurrent changes. A key that already exists is not replaced. `description` must be empty because Vault KV v2 has no equivalent first-class secret description. Values are stored as text without implicit base64 conversion, so invalid UTF-8 payloads are rejected.
- Cloud.ru Secret Manager interprets `path` as the logical secret path, requires `key` to be empty, and persists the optional `description`. When `CLOUDRU_ROOT_FOLDER_OMIT_PREFIX=true`, the configured root is added for the provider request and omitted from the returned logical path. When it is `false`, the request path must include the configured root. Paths outside the configured scope or containing traversal segments are rejected. Existing provider defaults, including the managed KMS key, are retained. Parent folders must already exist when required by Cloud.ru.

The intended integration is an explicit two-step flow:

```text
swarm-deploy -> Secrets.Create -> external provider
swarm-deploy -> Controller.Sync -> Docker Swarm
```

This separation lets the caller distinguish a successful external create from a later synchronization failure.

The gRPC listener does not add authentication, authorization, or TLS. It is intended for the existing internal integration model on a shared Swarm overlay network. User-facing access remains protected by `swarm-deploy` BasicAuth or its configured proxy. Do not publish this listener directly to untrusted networks.

The server also registers the standard `grpc.health.v1.Health` service. Overall health, `swarm_deploy.cloud_secrets.Controller`, and `swarm_deploy.cloud_secrets.Secrets` are reported as `SERVING` while the server is accepting requests.
