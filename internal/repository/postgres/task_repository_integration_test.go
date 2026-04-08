package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	taskdomain "example.com/taskservice/internal/domain/task"
)

func TestMigrationBackfillLegacyTasks(t *testing.T) {
	testWithTempDatabase(t, true, func(ctx context.Context, testDSN string) {
		pool, err := pgxpool.New(ctx, testDSN)
		if err != nil {
			t.Fatalf("open pool: %v", err)
		}
		defer pool.Close()

		var (
			templateID   int64
			scheduledFor time.Time
			kind         string
		)
		err = pool.QueryRow(ctx, `
			SELECT t.template_id, t.scheduled_for, tpl.kind
			FROM tasks t
			JOIN task_templates tpl ON tpl.id = t.template_id
			LIMIT 1
		`).Scan(&templateID, &scheduledFor, &kind)
		if err != nil {
			t.Fatalf("query backfilled row: %v", err)
		}

		if templateID == 0 {
			t.Fatal("expected template_id to be set by migration")
		}
		if kind != string(taskdomain.RecurrenceOneTime) {
			t.Fatalf("expected kind=one_time, got %s", kind)
		}
		if scheduledFor.Format("2006-01-02") != "2026-04-05" {
			t.Fatalf("expected scheduled_for=2026-04-05, got %s", scheduledFor.Format("2006-01-02"))
		}
	})
}

func TestRepositoryEnsureFutureInstancesOnConflict(t *testing.T) {
	testWithTempDatabase(t, false, func(ctx context.Context, testDSN string) {
		pool, err := pgxpool.New(ctx, testDSN)
		if err != nil {
			t.Fatalf("open pool: %v", err)
		}
		defer pool.Close()

		repo := New(pool)
		now := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
		tpl, err := repo.CreateTemplate(ctx, &taskdomain.Template{
			Title:          "Daily rounds",
			Description:    "Inspect patients",
			DefaultStatus:  taskdomain.StatusNew,
			RecurrenceKind: taskdomain.RecurrenceDaily,
			Recurrence:     taskdomain.RecurrenceRule{EveryNDays: 1},
			StartDate:      time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			Active:         true,
			GeneratedUntil: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		if err != nil {
			t.Fatalf("create template: %v", err)
		}

		targetDate := time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC)
		if err := repo.EnsureFutureInstances(ctx, *tpl, targetDate, []time.Time{targetDate}, taskdomain.OriginGenerated); err != nil {
			t.Fatalf("ensure instances first: %v", err)
		}
		if err := repo.EnsureFutureInstances(ctx, *tpl, targetDate, []time.Time{targetDate}, taskdomain.OriginGenerated); err != nil {
			t.Fatalf("ensure instances second: %v", err)
		}

		var count int
		err = pool.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE template_id = $1 AND scheduled_for = $2`, tpl.ID, targetDate).Scan(&count)
		if err != nil {
			t.Fatalf("count tasks: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected exactly one instance for same template/date, got %d", count)
		}
	})
}

func testWithTempDatabase(t *testing.T, withLegacySeed bool, fn func(ctx context.Context, testDSN string)) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	adminDSN := os.Getenv("TEST_DATABASE_ADMIN_DSN")
	if adminDSN == "" {
		adminDSN = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	adminConn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Skipf("skip integration test: cannot connect to postgres admin dsn (%v)", err)
	}
	defer adminConn.Close(ctx)

	dbName := fmt.Sprintf("taskservice_it_%d", time.Now().UnixNano())
	if _, err := adminConn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)); err != nil {
		t.Fatalf("create temp db: %v", err)
	}

	cleanup := func() {
		_, _ = adminConn.Exec(context.Background(), `
			SELECT pg_terminate_backend(pid)
			FROM pg_stat_activity
			WHERE datname = $1 AND pid <> pg_backend_pid()
		`, dbName)
		_, _ = adminConn.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, dbName))
	}
	defer cleanup()

	testDSN, err := replaceDBName(adminDSN, dbName)
	if err != nil {
		t.Fatalf("build temp dsn: %v", err)
	}

	testConn, err := pgx.Connect(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect temp db: %v", err)
	}

	if withLegacySeed {
		if _, err := testConn.Exec(ctx, `
			CREATE TABLE tasks (
				id BIGSERIAL PRIMARY KEY,
				title TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			INSERT INTO tasks (title, description, status, created_at, updated_at)
			VALUES ('Legacy task', 'legacy', 'new', '2026-04-05T10:00:00Z', '2026-04-05T10:00:00Z');
		`); err != nil {
			testConn.Close(ctx)
			t.Fatalf("seed legacy schema: %v", err)
		}
	}

	migrationPath := filepath.Join("..", "..", "..", "migrations", "0001_create_tasks.up.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	if err != nil {
		testConn.Close(ctx)
		t.Fatalf("read migration: %v", err)
	}
	if _, err := testConn.Exec(ctx, string(migrationSQL)); err != nil {
		testConn.Close(ctx)
		t.Fatalf("apply migration: %v", err)
	}

	if err := testConn.Close(ctx); err != nil {
		t.Fatalf("close temp conn: %v", err)
	}

	fn(ctx, testDSN)
}

func replaceDBName(dsn, dbName string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + dbName
	return parsed.String(), nil
}
