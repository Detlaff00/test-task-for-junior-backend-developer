CREATE TABLE IF NOT EXISTS task_templates (
	id BIGSERIAL PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	default_status TEXT NOT NULL,
	kind TEXT NOT NULL,
	rule_json JSONB NOT NULL DEFAULT '{}'::jsonb,
	start_date DATE NOT NULL,
	all_day BOOLEAN NOT NULL DEFAULT TRUE,
	start_time TEXT,
	end_time TEXT,
	active BOOLEAN NOT NULL DEFAULT TRUE,
	generated_until DATE NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tasks (
	id BIGSERIAL PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	all_day BOOLEAN NOT NULL DEFAULT TRUE,
	start_time TEXT,
	end_time TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS template_id BIGINT;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS scheduled_for DATE;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS origin TEXT;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS all_day BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS start_time TEXT;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS end_time TEXT;
ALTER TABLE task_templates ADD COLUMN IF NOT EXISTS all_day BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE task_templates ADD COLUMN IF NOT EXISTS start_time TEXT;
ALTER TABLE task_templates ADD COLUMN IF NOT EXISTS end_time TEXT;

DO $$
DECLARE
	rec RECORD;
	tpl_id BIGINT;
BEGIN
	FOR rec IN SELECT id, title, description, status, created_at FROM tasks WHERE template_id IS NULL LOOP
		INSERT INTO task_templates (
			title, description, default_status, kind, rule_json,
			start_date, all_day, start_time, end_time,
			active, generated_until, created_at, updated_at
		)
		VALUES (
			rec.title,
			rec.description,
			rec.status,
			'one_time',
			'{}'::jsonb,
			(rec.created_at AT TIME ZONE 'UTC')::date,
			TRUE,
			NULL,
			NULL,
			TRUE,
			(rec.created_at AT TIME ZONE 'UTC')::date,
			rec.created_at,
			rec.created_at
		)
		RETURNING id INTO tpl_id;

		UPDATE tasks
		SET template_id = tpl_id,
			scheduled_for = (rec.created_at AT TIME ZONE 'UTC')::date,
			all_day = TRUE,
			start_time = NULL,
			end_time = NULL,
			origin = 'manual'
		WHERE id = rec.id;
	END LOOP;
END $$;

ALTER TABLE tasks ALTER COLUMN template_id SET NOT NULL;
ALTER TABLE tasks ALTER COLUMN scheduled_for SET NOT NULL;
ALTER TABLE tasks ALTER COLUMN origin SET NOT NULL;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'tasks_template_id_fkey'
	) THEN
		ALTER TABLE tasks
			ADD CONSTRAINT tasks_template_id_fkey
			FOREIGN KEY (template_id) REFERENCES task_templates(id);
	END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks (status);
CREATE INDEX IF NOT EXISTS idx_tasks_scheduled_for ON tasks (scheduled_for DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_tasks_template_scheduled_for ON tasks (template_id, scheduled_for);
CREATE INDEX IF NOT EXISTS idx_task_templates_generated_until ON task_templates (generated_until) WHERE active = TRUE;
