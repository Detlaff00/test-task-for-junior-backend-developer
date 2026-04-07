package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"

	taskdomain "example.com/taskservice/internal/domain/task"
	taskusecase "example.com/taskservice/internal/usecase/task"
)

type usecaseStub struct {
	createFn  func(ctx context.Context, input taskusecase.CreateInput) (*taskdomain.Task, error)
	getByIDFn func(ctx context.Context, id int64) (*taskdomain.Task, error)
	updateFn  func(ctx context.Context, id int64, input taskusecase.UpdateInput) (*taskdomain.Task, error)
	deleteFn  func(ctx context.Context, id int64) error
	listFn    func(ctx context.Context) ([]taskdomain.Task, error)
}

func (s usecaseStub) Create(ctx context.Context, input taskusecase.CreateInput) (*taskdomain.Task, error) {
	if s.createFn == nil {
		return nil, errors.New("unexpected call")
	}
	return s.createFn(ctx, input)
}

func (s usecaseStub) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	if s.getByIDFn == nil {
		return nil, errors.New("unexpected call")
	}
	return s.getByIDFn(ctx, id)
}

func (s usecaseStub) Update(ctx context.Context, id int64, input taskusecase.UpdateInput) (*taskdomain.Task, error) {
	if s.updateFn == nil {
		return nil, errors.New("unexpected call")
	}
	return s.updateFn(ctx, id, input)
}

func (s usecaseStub) Delete(ctx context.Context, id int64) error {
	if s.deleteFn == nil {
		return errors.New("unexpected call")
	}
	return s.deleteFn(ctx, id)
}

func (s usecaseStub) List(ctx context.Context) ([]taskdomain.Task, error) {
	if s.listFn == nil {
		return nil, errors.New("unexpected call")
	}
	return s.listFn(ctx)
}

func TestTaskHandlerCreate(t *testing.T) {
	t.Parallel()

	captured := taskusecase.CreateInput{}
	h := NewTaskHandler(usecaseStub{
		createFn: func(_ context.Context, input taskusecase.CreateInput) (*taskdomain.Task, error) {
			captured = input
			return &taskdomain.Task{
				ID:             11,
				TemplateID:     3,
				Title:          input.Title,
				Description:    input.Description,
				Status:         input.Status,
				RecurrenceKind: taskdomain.RecurrenceDaily,
				Recurrence:     taskdomain.RecurrenceRule{EveryNDays: 2},
				ScheduledFor:   mustDate("2026-04-07"),
				Origin:         taskdomain.OriginManual,
				CreatedAt:      time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	})

	payload := []byte(`{"title":"Follow up","description":"Call patient","status":"new","recurrence_kind":"daily","recurrence":{"every_n_days":2},"start_date":"2026-04-07"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}
	if captured.RecurrenceKind != taskdomain.RecurrenceDaily || captured.Recurrence.EveryNDays == nil || *captured.Recurrence.EveryNDays != 2 {
		t.Fatalf("recurrence mapping failed: %+v", captured)
	}

	var body taskDTO
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ScheduledFor != "2026-04-07" || body.TemplateID != 3 {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestTaskHandlerGetByIDNotFound(t *testing.T) {
	t.Parallel()

	h := NewTaskHandler(usecaseStub{
		getByIDFn: func(_ context.Context, _ int64) (*taskdomain.Task, error) {
			return nil, taskdomain.ErrNotFound
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/10", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "10"})
	w := httptest.NewRecorder()

	h.GetByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

func TestTaskHandlerCreateInvalidInput(t *testing.T) {
	t.Parallel()

	h := NewTaskHandler(usecaseStub{
		createFn: func(_ context.Context, _ taskusecase.CreateInput) (*taskdomain.Task, error) {
			return nil, taskusecase.ErrInvalidInput
		},
	})

	payload := []byte(`{"title":"X"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestTaskHandlerList(t *testing.T) {
	t.Parallel()

	h := NewTaskHandler(usecaseStub{
		listFn: func(_ context.Context) ([]taskdomain.Task, error) {
			return []taskdomain.Task{{
				ID:             1,
				TemplateID:     100,
				Title:          "A",
				Description:    "B",
				Status:         taskdomain.StatusNew,
				RecurrenceKind: taskdomain.RecurrenceOneTime,
				ScheduledFor:   mustDate("2026-04-07"),
				Origin:         taskdomain.OriginManual,
				CreatedAt:      time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
			}}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body []taskDTO
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) != 1 || body[0].TemplateID != 100 {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func mustDate(v string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", v, time.UTC)
	if err != nil {
		panic(err)
	}
	return t
}
