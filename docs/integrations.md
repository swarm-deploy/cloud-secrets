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

## Service

The protobuf package is `swarm_deploy.cloud_secrets` and exposes the `Controller` service:

```proto
service Controller {
  rpc GetInfo(GetInfoRequest) returns (GetInfoResponse);
  rpc Sync(SyncRequest) returns (SyncResponse);
}
```

`GetInfo` returns the running version, configured provider name and link when available, the timestamp of the last successful synchronization known by the current process, and the next scheduled interval synchronization time. `last_sync_at` is absent after process start until the first successful synchronization and is not persisted across restarts. `next_sync_at` follows the interval timer; manual gRPC and SIGHUP synchronizations do not reset it.

`Sync` invokes the same synchronization path used by interval and SIGHUP triggers. If another synchronization is already running, the RPC returns gRPC status `ABORTED`.

The response summarizes created, updated, removed, and unchanged logical secrets. Removed historical secret versions are not included in `removed`.

The server also registers the standard `grpc.health.v1.Health` service. Overall health and `swarm_deploy.cloud_secrets.Controller` are reported as `SERVING` while the server is accepting requests.
