# Monitoring

[На русском](monitoring_ru.md)

**cloud-secrets** exports Prometheus metrics at `:8000/metrics`.

Application metrics use the `cloud_secrets` namespace.

[Grafana dashboard](./../grafana-dashboard.json)

## cloud-secrets Metrics

| Metric                                            | Type      | Labels                              | Description                                                                                                            |
|---------------------------------------------------|-----------|-------------------------------------|------------------------------------------------------------------------------------------------------------------------|
| `cloud_secrets_build_info`                        | Gauge     | `version`, `date`                   | Build information. The metric value is always `1` for the current build.                                               |
| `cloud_secrets_syncs_runs_total`                  | Counter   | `trigger`                           | Number of synchronization runs, grouped by trigger source.                                                             |
| `cloud_secrets_syncs_last_sync_at_unix`           | Gauge     | -                                   | Unix timestamp of the last completed synchronization run.                                                              |
| `cloud_secrets_secrets_created_total`             | Counter   | -                                   | Number of Docker Swarm secrets created by synchronization.                                                             |
| `cloud_secrets_secrets_updated_total`             | Counter   | -                                   | Number of Docker Swarm secrets updated by synchronization.                                                             |
| `cloud_secrets_secrets_removed_total`             | Counter   | -                                   | Number of Docker Swarm secrets removed by synchronization.                                                             |
| `cloud_secrets_secrets_removed_versions_total`    | Counter   | -                                   | Number of old Docker Swarm secret versions removed by synchronization.                                                 |
| `cloud_secrets_docker_requests_total`             | Counter   | `operation`                         | Number of Docker API requests, grouped by operation.                                                                   |
| `cloud_secrets_docker_request_duration_seconds`   | Histogram | `operation`, `le` for bucket series | Docker API request duration in seconds, grouped by operation. Exported as `_bucket`, `_sum`, and `_count` series.      |
| `cloud_secrets_provider_requests_total`           | Counter   | `operation`                         | Number of secret provider requests, grouped by operation.                                                              |
| `cloud_secrets_provider_request_duration_seconds` | Histogram | `operation`, `le` for bucket series | Secret provider request duration in seconds, grouped by operation. Exported as `_bucket`, `_sum`, and `_count` series. |

The `/metrics` endpoint also includes standard Go runtime, process, gopipe, and Prometheus HTTP handler metrics registered by the Prometheus Go client.
