# Monitoring

[На русском](monitoring_ru.md)

**cloud-secrets** exports Prometheus metrics at `:8000/metrics`.

Application metrics use the `cloud_secrets` namespace.

[Grafana dashboard](./../grafana-dashboard.json)

## cloud-secrets metrics

| Metric                                            | Type      | Labels            | Description                                                              |
|---------------------------------------------------|-----------|-------------------|--------------------------------------------------------------------------|
| `cloud_secrets_build_info`                        | Gauge     | `version`, `date` | Build information. The metric value is always `1` for the current build. |
| `cloud_secrets_syncs_runs_total`                  | Counter   | `trigger`         | Number of synchronization runs, grouped by trigger source.               |
| `cloud_secrets_syncs_last_sync_at_unix`           | Gauge     | -                 | Unix timestamp of the last completed synchronization run.                |
| `cloud_secrets_secrets_created_total`             | Counter   | -                 | Number of Docker Swarm secrets created by synchronization.               |
| `cloud_secrets_secrets_updated_total`             | Counter   | -                 | Number of Docker Swarm secrets updated by synchronization.               |
| `cloud_secrets_secrets_removed_total`             | Counter   | -                 | Number of Docker Swarm secrets removed by synchronization.               |
| `cloud_secrets_secrets_removed_versions_total`    | Counter   | -                 | Number of old Docker Swarm secret versions removed by synchronization.   |
| `cloud_secrets_docker_requests_total`             | Counter   | `operation`       | Number of Docker API requests, grouped by operation.                     |
| `cloud_secrets_docker_request_duration_seconds`   | Histogram | `operation`, `le` | Docker API request duration in seconds, grouped by operation.            |
| `cloud_secrets_provider_requests_total`           | Counter   | `operation`       | Number of secret provider requests, grouped by operation.                |
| `cloud_secrets_provider_request_duration_seconds` | Histogram | `operation`, `le` | Secret provider request duration in seconds, grouped by operation.       |

### Sync triggers

| Value      | Description                                       |
| ---------- | ------------------------------------------------- |
| `interval` | Synchronization started by the refresh interval.  |
| `sighup`   | Synchronization started after receiving `SIGHUP`. |

### Docker operations

| Value                   |
|-------------------------|
| `list_secrets`          |
| `create_secret`         |
| `create_secret_version` |
| `remove_secret`         |
| `list_services`         |
| `update_service`        |

### Provider operations

| Value                |
|----------------------|
| `list_secrets`       |
| `get_secret_payload` |

## Pipeline metrics

The synchronization pipeline exposes additional metrics provided by
[gopipe](https://github.com/ArtARTs36/gopipe/blob/master/docs/monitoring.md).

cloud-secrets uses the `sync_secrets` pipeline with the following steps:

* `load_swarm_state`
* `load_external_state`
* `process_secrets`
* `apply_service_updates`
* `remove_old_secret_versions`
* `restore_parent_secrets`
* `cleanup_orphaned_secrets`

See the [gopipe monitoring documentation](https://github.com/ArtARTs36/gopipe/blob/master/docs/monitoring.md)
for metric names and semantics.

## Runtime metrics

The `/metrics` endpoint also includes the standard Go runtime, process, and
Prometheus HTTP handler metrics registered by the Prometheus Go client.
