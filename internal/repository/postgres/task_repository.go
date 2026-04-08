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
			start_date, all_day, start_time, end_time,
			active, generated_until, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, title, description, default_status, kind, rule_json,
			start_date, all_day, start_time, end_time,
			active, generated_until, created_at, updated_at
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
		template.AllDay,
		template.StartTime,
		template.EndTime,
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
			all_day = $7,
			start_time = $8,
			end_time = $9,
			active = $10,
			generated_until = $11,
			updated_at = $12
		WHERE id = $13
		RETURNING id, title, description, default_status, kind, rule_json,
			start_date, all_day, start_time, end_time,
			active, generated_until, created_at, updated_at
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
		template.AllDay,
		template.StartTime,
		template.EndTime,
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

func (r *Repository) GetTemplateByID(ctx context.Context, templateID int64) (*taskdomain.Template, error) {
	const query = `
		SELECT id, title, description, default_status, kind, rule_json,
			start_date, all_day, start_time, end_time,
			active, generated_until, created_at, updated_at
		FROM task_templates
		WHERE id = $1
	`

	row := r.pool.QueryRow(ctx, query, templateID)
	tpl, err := scanTemplate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}
		return nil, err
	}

	return tpl, nil
}

func (r *Repository) ListActiveTemplates(ctx context.Context) ([]taskdomain.Template, error) {
	const query = `
		SELECT id, title, description, default_status, kind, rule_json,
			start_date, all_day, start_time, end_time,
			active, generated_until, created_at, updated_at
		FROM task_templates
		WHERE active = true
		ORDER BY id ASC
	`

	rows, err := r.pool.Query(ctx, query)
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

func (r *Repository) ReplaceFutureInstances(
	ctx context.Context,
	template taskdomain.Template,
	fromDate time.Time,
	dates []time.Time,
	origin taskdomain.Origin,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const deleteQuery = `
		DELETE FROM tasks
		WHERE template_id = $1
			AND scheduled_for >= $2
			AND origin = $3
	`
	if _, err := tx.Exec(ctx, deleteQuery, template.ID, fromDate, taskdomain.OriginGenerated); err != nil {
		return err
	}

	if len(dates) > 0 {
		const insertQuery = `
			INSERT INTO tasks (
				template_id, title, description, status, scheduled_for,
				all_day, start_time, end_time,
				origin, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
			ON CONFLICT (template_id, scheduled_for) DO NOTHING
		`

		now := time.Now().UTC()
		batch := &pgx.Batch{}
		for _, date := range dates {
			batch.Queue(
				insertQuery,
				template.ID,
				template.Title,
				template.Description,
				template.DefaultStatus,
				date,
				template.AllDay,
				template.StartTime,
				template.EndTime,
				origin,
				now,
			)
		}

		results := tx.SendBatch(ctx, batch)
		for range dates {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return err
			}
		}
		if err := results.Close(); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE task_templates SET generated_until = $1, updated_at = NOW() WHERE id = $2`, fromDate, template.ID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (r *Repository) EnsureFutureInstances(
	ctx context.Context,
	template taskdomain.Template,
	_ time.Time,
	dates []time.Time,
	origin taskdomain.Origin,
) error {
	if len(dates) == 0 {
		return nil
	}

	const query = `
		INSERT INTO tasks (
			template_id, title, description, status, scheduled_for,
			all_day, start_time, end_time,
			origin, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
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
			template.AllDay,
			template.StartTime,
			template.EndTime,
			origin,
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

func (r *Repository) DeactivateTemplate(ctx context.Context, templateID int64) error {
	const query = `
		UPDATE task_templates
		SET active = false,
			updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.pool.Exec(ctx, query, templateID)
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
			t.scheduled_for, t.all_day, t.start_time, t.end_time,
			t.origin, t.created_at, t.updated_at,
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

func (r *Repository) GetNearestByTemplateID(ctx context.Context, templateID int64, fromDate time.Time) (*taskdomain.Task, error) {
	const query = `
		SELECT t.id, t.template_id, t.title, t.description, t.status,
			t.scheduled_for, t.all_day, t.start_time, t.end_time,
			t.origin, t.created_at, t.updated_at,
			tpl.kind, tpl.rule_json, tpl.start_date
		FROM tasks t
		JOIN task_templates tpl ON tpl.id = t.template_id
		WHERE t.template_id = $1
			AND t.scheduled_for >= $2
		ORDER BY t.scheduled_for ASC, t.id ASC
		LIMIT 1
	`

	row := r.pool.QueryRow(ctx, query, templateID, fromDate)
	item, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}
		return nil, err
	}

	return item, nil
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
			t.scheduled_for, t.all_day, t.start_time, t.end_time,
			t.origin, t.created_at, t.updated_at,
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
		&task.AllDay,
		&task.StartTime,
		&task.EndTime,
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
		&tpl.AllDay,
		&tpl.StartTime,
		&tpl.EndTime,
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
