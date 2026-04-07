package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateTemplate(ctx context.Context, template *taskdomain.Template) (*taskdomain.Template, error) {
	ruleJSON, err := json.Marshal(template.Recurrence)
	if err != nil {
		return nil, err
	}

	const query = `
		INSERT INTO task_templates (
			title, description, default_status, kind, rule_json,
			start_date, active, generated_until, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10)
		RETURNING id, title, description, default_status, kind, rule_json,
			start_date, active, generated_until, created_at, updated_at
	`

	row := r.pool.QueryRow(
		ctx,
		query,
		template.Title,
		template.Description,
		template.DefaultStatus,
		template.RecurrenceKind,
		string(ruleJSON),
		template.StartDate,
		template.Active,
		template.GeneratedUntil,
		template.CreatedAt,
		template.UpdatedAt,
	)

	return scanTemplate(row)
}

func (r *Repository) UpdateTemplate(ctx context.Context, template *taskdomain.Template) (*taskdomain.Template, error) {
	ruleJSON, err := json.Marshal(template.Recurrence)
	if err != nil {
		return nil, err
	}

	const query = `
		UPDATE task_templates
		SET title = $1,
			description = $2,
			default_status = $3,
			kind = $4,
			rule_json = $5::jsonb,
			start_date = $6,
			active = $7,
			generated_until = $8,
			updated_at = $9
		WHERE id = $10
		RETURNING id, title, description, default_status, kind, rule_json,
			start_date, active, generated_until, created_at, updated_at
	`

	row := r.pool.QueryRow(
		ctx,
		query,
		template.Title,
		template.Description,
		template.DefaultStatus,
		template.RecurrenceKind,
		string(ruleJSON),
		template.StartDate,
		template.Active,
		template.GeneratedUntil,
		template.UpdatedAt,
		template.ID,
	)

	updated, err := scanTemplate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}
		return nil, err
	}

	return updated, nil
}

func (r *Repository) ListTemplatesToGenerate(ctx context.Context, today time.Time) ([]taskdomain.Template, error) {
	const query = `
		SELECT id, title, description, default_status, kind, rule_json,
			start_date, active, generated_until, created_at, updated_at
		FROM task_templates
		WHERE active = true
			AND generated_until < $1
		ORDER BY id ASC
	`

	rows, err := r.pool.Query(ctx, query, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := make([]taskdomain.Template, 0)
	for rows.Next() {
		tpl, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, *tpl)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return templates, nil
}

func (r *Repository) CreateInstances(ctx context.Context, template taskdomain.Template, dates []time.Time, origin taskdomain.Origin) error {
	const query = `
		INSERT INTO tasks (
			template_id, title, description, status,
			scheduled_for, origin, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (template_id, scheduled_for) DO NOTHING
	`

	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, date := range dates {
		batch.Queue(
			query,
			template.ID,
			template.Title,
			template.Description,
			template.DefaultStatus,
			date,
			origin,
			now,
			now,
		)
	}

	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()

	for range dates {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repository) SetTemplateGeneratedUntil(ctx context.Context, templateID int64, generatedUntil time.Time) error {
	const query = `
		UPDATE task_templates
		SET generated_until = $1,
			updated_at = NOW()
		WHERE id = $2
	`

	result, err := r.pool.Exec(ctx, query, generatedUntil, templateID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return taskdomain.ErrNotFound
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	const query = `
		SELECT t.id, t.template_id, t.title, t.description, t.status,
			t.scheduled_for, t.origin, t.created_at, t.updated_at,
			tpl.kind, tpl.rule_json, tpl.start_date
		FROM tasks t
		JOIN task_templates tpl ON tpl.id = t.template_id
		WHERE t.id = $1
	`

	row := r.pool.QueryRow(ctx, query, id)
	found, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}
		return nil, err
	}

	return found, nil
}

func (r *Repository) Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	const query = `
		UPDATE tasks
		SET title = $1,
			description = $2,
			status = $3,
			updated_at = $4
		WHERE id = $5
		RETURNING id
	`

	var id int64
	row := r.pool.QueryRow(ctx, query, task.Title, task.Description, task.Status, task.UpdatedAt, task.ID)
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}
		return nil, err
	}

	return r.GetByID(ctx, id)
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	const query = `DELETE FROM tasks WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return taskdomain.ErrNotFound
	}

	return nil
}

func (r *Repository) List(ctx context.Context) ([]taskdomain.Task, error) {
	const query = `
		SELECT t.id, t.template_id, t.title, t.description, t.status,
			t.scheduled_for, t.origin, t.created_at, t.updated_at,
			tpl.kind, tpl.rule_json, tpl.start_date
		FROM tasks t
		JOIN task_templates tpl ON tpl.id = t.template_id
		ORDER BY t.scheduled_for DESC, t.id DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]taskdomain.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *task)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

func (r *Repository) GetLatestByTemplateID(ctx context.Context, templateID int64) (*taskdomain.Task, error) {
	const query = `
		SELECT t.id, t.template_id, t.title, t.description, t.status,
			t.scheduled_for, t.origin, t.created_at, t.updated_at,
			tpl.kind, tpl.rule_json, tpl.start_date
		FROM tasks t
		JOIN task_templates tpl ON tpl.id = t.template_id
		WHERE t.template_id = $1
		ORDER BY t.scheduled_for DESC, t.id DESC
		LIMIT 1
	`

	row := r.pool.QueryRow(ctx, query, templateID)
	item, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}
		return nil, err
	}

	return item, nil
}

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (*taskdomain.Task, error) {
	var (
		task      taskdomain.Task
		status    string
		kind      string
		rawRule   []byte
		rawOrigin string
	)

	if err := scanner.Scan(
		&task.ID,
		&task.TemplateID,
		&task.Title,
		&task.Description,
		&status,
		&task.ScheduledFor,
		&rawOrigin,
		&task.CreatedAt,
		&task.UpdatedAt,
		&kind,
		&rawRule,
		&task.TemplateStart,
	); err != nil {
		return nil, err
	}

	task.Status = taskdomain.Status(status)
	task.Origin = taskdomain.Origin(rawOrigin)
	task.RecurrenceKind = taskdomain.RecurrenceKind(kind)
	if len(rawRule) > 0 {
		if err := json.Unmarshal(rawRule, &task.Recurrence); err != nil {
			return nil, err
		}
	}

	return &task, nil
}

func scanTemplate(scanner taskScanner) (*taskdomain.Template, error) {
	var (
		tpl     taskdomain.Template
		status  string
		kind    string
		rawRule []byte
	)

	if err := scanner.Scan(
		&tpl.ID,
		&tpl.Title,
		&tpl.Description,
		&status,
		&kind,
		&rawRule,
		&tpl.StartDate,
		&tpl.Active,
		&tpl.GeneratedUntil,
		&tpl.CreatedAt,
		&tpl.UpdatedAt,
	); err != nil {
		return nil, err
	}

	tpl.DefaultStatus = taskdomain.Status(status)
	tpl.RecurrenceKind = taskdomain.RecurrenceKind(kind)
	if len(rawRule) > 0 {
		if err := json.Unmarshal(rawRule, &tpl.Recurrence); err != nil {
			return nil, err
		}
	}

	return &tpl, nil
}
