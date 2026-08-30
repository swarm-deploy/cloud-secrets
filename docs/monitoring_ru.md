# Monitoring

[In English](monitoring.md)

**cloud-secrets** экспортирует Prometheus-метрики на `:8000/metrics`.

Метрики приложения используют namespace `cloud_secrets`.

[Grafana dashboard](./../grafana-dashboard.json)

## Метрики cloud-secrets

| Метрика                                           | Тип       | Labels                             | Описание                                                                                                                                    |
|---------------------------------------------------|-----------|------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------|
| `cloud_secrets_build_info`                        | Gauge     | `version`, `date`                  | Информация о сборке. Значение метрики всегда `1` для текущей сборки.                                                                        |
| `cloud_secrets_syncs_runs_total`                  | Counter   | `trigger`                          | Количество запусков синхронизации, сгруппированное по источнику запуска.                                                                    |
| `cloud_secrets_syncs_last_sync_at_unix`           | Gauge     | -                                  | Unix timestamp последнего завершенного запуска синхронизации.                                                                               |
| `cloud_secrets_secrets_created_total`             | Counter   | -                                  | Количество Docker Swarm secrets, созданных синхронизацией.                                                                                  |
| `cloud_secrets_secrets_updated_total`             | Counter   | -                                  | Количество Docker Swarm secrets, обновленных синхронизацией.                                                                                |
| `cloud_secrets_secrets_removed_total`             | Counter   | -                                  | Количество Docker Swarm secrets, удаленных синхронизацией.                                                                                  |
| `cloud_secrets_secrets_removed_versions_total`    | Counter   | -                                  | Количество старых версий Docker Swarm secrets, удаленных синхронизацией.                                                                    |
| `cloud_secrets_docker_requests_total`             | Counter   | `operation`                        | Количество запросов к Docker API, сгруппированное по операции.                                                                              |
| `cloud_secrets_docker_request_duration_seconds`   | Histogram | `operation`, `le` для bucket-серий | Длительность запросов к Docker API в секундах, сгруппированная по операции. Экспортируется как серии `_bucket`, `_sum` и `_count`.          |
| `cloud_secrets_provider_requests_total`           | Counter   | `operation`                        | Количество запросов к провайдеру секретов, сгруппированное по операции.                                                                     |
| `cloud_secrets_provider_request_duration_seconds` | Histogram | `operation`, `le` для bucket-серий | Длительность запросов к провайдеру секретов в секундах, сгруппированная по операции. Экспортируется как серии `_bucket`, `_sum` и `_count`. |

Endpoint `/metrics` также включает стандартные метрики Go runtime, process, gopipe и Prometheus HTTP handler, которые регистрирует Prometheus Go client.
