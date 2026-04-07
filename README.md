# Task Service

Сервис для управления задачами медицинского персонала с HTTP API на Go.

## Что реализовано в тестовом задании

В проект добавлена полноценная поддержка периодических задач.

### 1) Новая модель данных

Вместо хранения только одной сущности `tasks` введена двухуровневая модель:

- `task_templates` — шаблон (правило периодичности)
- `tasks` — экземпляры задач (конкретные задачи в трекере)

Для `tasks` добавлены:

- `template_id`
- `scheduled_for`
- `origin` (`manual`/`generated`)

Также добавлены индексы и уникальность `(template_id, scheduled_for)` для идемпотентной генерации.

### 2) Поддержка периодичности

Добавлены типы `recurrence_kind`:

- `one_time`
- `daily` (`every_n_days >= 1`)
- `monthly_day` (`day_of_month` в диапазоне `1..30`)
- `specific_dates` (непустой массив уникальных дат `YYYY-MM-DD`)
- `monthly_parity` (`parity`: `odd` или `even`)

### 3) Генерация экземпляров

Реализована `on-demand` генерация в UTC:

- при `GET /api/v1/tasks`
- при `POST /api/v1/tasks`
- при обновлении правила через `PUT /api/v1/tasks/{id}`

Генерация выполняется до текущего дня включительно и идемпотентна.

### 4) API и документация

Расширен контракт существующих эндпоинтов `/api/v1/tasks`:

- в request добавлены `recurrence_kind`, `recurrence`, `start_date`
- в response добавлены `template_id`, `recurrence_kind`, `recurrence`, `scheduled_for`, `origin`

Обновлён OpenAPI: `internal/transport/http/docs/openapi.json`.

### 5) Тесты

Добавлены:

- unit-тесты usecase (валидация правил и генерация дат)
- HTTP-тесты хендлеров
- integration-тесты репозитория/Postgres (включая backfill миграции и `ON CONFLICT DO NOTHING`)

---

## Требования

- Go `1.23+`
- Docker и Docker Compose

## Быстрый запуск

```bash
docker compose down -v
docker compose up --build
```

После запуска сервис доступен по адресу `http://localhost:8080`.

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

---

## Как проверить

### 1) Прогон тестов

```bash
go test ./...
```

### 2) Проверка создания периодической задачи

Пример: ежедневная задача каждые 2 дня с даты `2026-04-01`.

```bash
curl -sS -X POST http://localhost:8080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "title": "Daily patient follow-up",
    "description": "Call post-op patients",
    "status": "new",
    "recurrence_kind": "daily",
    "recurrence": {"every_n_days": 2},
    "start_date": "2026-04-01"
  }'
```

Ожидание:

- ответ `201`
- в ответе есть `template_id`, `scheduled_for`, `recurrence_kind`, `recurrence`

### 3) Проверка списка задач

```bash
curl -sS http://localhost:8080/api/v1/tasks
```

Ожидание:

- возвращаются экземпляры задач (не шаблоны)
- сортировка по `scheduled_for DESC, id DESC`

### 4) Проверка валидации

Пример некорректного `monthly_day`:

```bash
curl -i -X POST http://localhost:8080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "title": "Bad monthly",
    "status": "new",
    "recurrence_kind": "monthly_day",
    "recurrence": {"day_of_month": 31}
  }'
```

Ожидание:

- `400 Bad Request`
- сообщение об ошибке валидации

---

## Принятые допущения

- Внутренняя модель: `task_templates` + `tasks` (экземпляры).
- Публичный API `/tasks` работает только с экземплярами задач.
- Все расчёты периодичности выполняются в UTC.
- Горизонт генерации: до текущего дня включительно (без look-ahead).
- Изменение правила влияет только на будущие экземпляры.
- Уже созданные экземпляры не удаляются при изменении/удалении шаблона.
