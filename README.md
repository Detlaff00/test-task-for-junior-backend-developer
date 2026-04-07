# Task Service

Сервис для управления задачами медицинского персонала с HTTP API на Go.

## Требования

- Go `1.23+`
- Docker и Docker Compose

## Быстрый запуск через Docker Compose

```bash
docker compose down -v
docker compose up --build
```

После запуска сервис доступен по адресу `http://localhost:8080`.

`docker compose down -v` рекомендован перед первым запуском после обновления схемы, так как SQL из `migrations/0001_create_tasks.up.sql` применяется только при инициализации пустого volume.

## Swagger

- UI: `http://localhost:8080/swagger/`
- OpenAPI JSON: `http://localhost:8080/swagger/openapi.json`

## API

Базовый префикс: `/api/v1`

Маршруты:

- `POST /tasks`
- `GET /tasks`
- `GET /tasks/{id}`
- `PUT /tasks/{id}`
- `DELETE /tasks/{id}`

### Периодичность

Поддерживаемые `recurrence_kind`:

- `one_time`
- `daily` (`every_n_days >= 1`)
- `monthly_day` (`day_of_month` в диапазоне `1..30`)
- `specific_dates` (непустой массив уникальных дат `YYYY-MM-DD`)
- `monthly_parity` (`parity`: `odd` или `even`)

`start_date` опционален. Если не передан, используется текущая UTC-дата.

## Принятые допущения

- Внутренняя модель состоит из `task_templates` (правила) и `tasks` (экземпляры).
- Публичный API `/tasks` работает только с экземплярами задач.
- Генерация экземпляров выполняется идемпотентно в UTC и только до текущего дня включительно (без look-ahead).
- Уже созданные экземпляры остаются неизменными при обновлении правила; новые генерируются по обновлённому правилу.
- Удаление экземпляра не удаляет шаблон и другие экземпляры.
- Удаление шаблонов не вынесено в публичный API; уже созданные экземпляры сохраняются.
