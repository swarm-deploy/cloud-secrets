# Monitoring

[In English](monitoring.md)

**cloud-secrets** экспортирует Prometheus-метрики на `:8000/metrics`.

Метрики приложения используют пространство имен `cloud_secrets`.

[Дашборд Grafana](./../grafana-dashboard.json)

## Метрики cloud-secrets

| Метрика                                           | Тип       | Labels            | Описание                                                                                  |
|---------------------------------------------------|-----------|-------------------|-------------------------------------------------------------------------------------------|
| `cloud_secrets_build_info`                        | Gauge     | `version`, `date` | Информация о сборке. Значение метрики всегда `1` для текущей сборки.                      |
| `cloud_secrets_syncs_runs_total`                  | Counter   | `trigger`         | Количество запусков синхронизации, сгруппированное по источнику запуска.                  |
| `cloud_secrets_syncs_last_sync_at_unix`           | Gauge     | -                 | Unix timestamp последнего завершенного запуска синхронизации.                             |
| `cloud_secrets_secrets_created_total`             | Counter   | -                 | Количество Docker Swarm secrets, созданных синхронизацией.                                |
| `cloud_secrets_secrets_updated_total`             | Counter   | -                 | Количество Docker Swarm secrets, обновленных синхронизацией.                              |
| `cloud_secrets_secrets_removed_total`             | Counter   | -                 | Количество Docker Swarm secrets, удаленных синхронизацией.                                |
| `cloud_secrets_secrets_removed_versions_total`    | Counter   | -                 | Количество старых версий Docker Swarm secrets, удаленных синхронизацией.                  |
| `cloud_secrets_docker_requests_total`             | Counter   | `operation`       | Количество запросов к Docker API, сгруппированное по операции.                            |
| `cloud_secrets_docker_request_duration_seconds`   | Histogram | `operation`, `le` | Длительность запросов к Docker API в секундах, сгруппированная по операции.               |
| `cloud_secrets_provider_requests_total`           | Counter   | `operation`       | Количество запросов к провайдеру секретов, сгруппированное по операции.                   |
| `cloud_secrets_provider_request_duration_seconds` | Histogram | `operation`, `le` | Длительность запросов к провайдеру секретов в секундах, сгруппированная по операции.      |

### Триггеры синхронизации

| Значение   | Описание                                      |
| ---------- | --------------------------------------------- |
| `interval` | Синхронизация запущена по интервалу обновления. |
| `sighup`   | Синхронизация запущена после получения `SIGHUP`. |

### Операции Docker

| Значение                |
|-------------------------|
| `list_secrets`          |
| `create_secret`         |
| `create_secret_version` |
| `remove_secret`         |
| `list_services`         |
| `update_service`        |

### Операции провайдера

| Значение             |
|----------------------|
| `list_secrets`       |
| `get_secret_payload` |

## Метрики пайплайна

Пайплайн синхронизации экспортирует дополнительные метрики, предоставляемые
[gopipe](https://github.com/ArtARTs36/gopipe/blob/master/docs/monitoring.md).

cloud-secrets использует пайплайн `sync_secrets` со следующими шагами:

* `load_swarm_state`
* `load_external_state`
* `process_secrets`
* `apply_service_updates`
* `remove_old_secret_versions`
* `restore_parent_secrets`
* `cleanup_orphaned_secrets`

Имена и семантику метрик см. в
[документации gopipe по мониторингу](https://github.com/ArtARTs36/gopipe/blob/master/docs/monitoring.md).

## Runtime-метрики

Эндпоинт `/metrics` также включает стандартные метрики Go runtime, process и
Prometheus HTTP handler, которые регистрирует Prometheus Go client.
