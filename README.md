# Currencies

Сервис асинхронных котировок валют

## Запуск

Всё (приложение, PostgreSQL, Jaeger, Prometheus, Grafana) поднимается одним
docker compose в общей сети:

```bash
cp .env.example .env       # настройки приложения (значения по умолчанию рабочие)
docker compose up -d --build
```

После старта:

| Что | Адрес |
|---|---|
| API | http://localhost:8080 |
| Swagger UI | http://localhost:8080/swagger/index.html |
| Health | http://localhost:8082/health |
| Metrics | http://localhost:8081/metrics |
| Jaeger UI (трейсы) | http://localhost:16686 |
| Prometheus | http://localhost:10501 |
| Grafana (`admin`/`admin`, datasource провижен автоматически) | http://localhost:10502 |
| PostgreSQL (для IDE/psql с хоста) | `localhost:10400`, `currencies-user`/`password`, БД `currencies` |

Схема БД создаётся автоматически: SQL-скрипты из `migrations/` монтируются в
`/docker-entrypoint-initdb.d` и выполняются при первой инициализации volume.
Схема применяется только на чистый volume; после изменения миграций:
`docker compose down -v && docker compose up -d --build`.

## API

| Метод | Путь | Описание |
|---|---|---|
| `POST` | `/api/v1/quotes?pair=EUR/MXN` | Зарегистрировать запрос обновления пары, вернуть `requestID` (202) |
| `GET` | `/api/v1/quotes/{requestID}` | Котировка по идентификатору запроса: цена + время (404, пока не готова) |
| `GET` | `/api/v1/quotes/last?pair=EUR/MXN` | Последнее значение котировки по паре |

Обе валюты пары должны быть из поддерживаемого набора `EXCHANGE_SUPPORTED`
(по умолчанию `EUR,MXN,USD`); любые комбинации из набора допустимы: `EUR/MXN`, `MXN/EUR`.