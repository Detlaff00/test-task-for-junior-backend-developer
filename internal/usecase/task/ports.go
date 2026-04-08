package task

import (
	"context"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type Repository interface {
	CreateTemplate(ctx context.Context, template *taskdomain.Template) (*taskdomain.Template, error)
	UpdateTemplate(ctx context.Context, template *taskdomain.Template) (*taskdomain.Template, error)
	GetTemplateByID(ctx context.Context, templateID int64) (*taskdomain.Template, error)
	ListActiveTemplates(ctx context.Context) ([]taskdomain.Template, error)
	ReplaceFutureInstances(ctx context.Context, template taskdomain.Template, fromDate time.Time, dates []time.Time, origin taskdomain.Origin) error
	EnsureFutureInstances(ctx context.Context, template taskdomain.Template, fromDate time.Time, dates []time.Time, origin taskdomain.Origin) error
	DeactivateTemplate(ctx context.Context, templateID int64) error

	GetByID(ctx context.Context, id int64) (*taskdomain.Task, error)
	GetNearestByTemplateID(ctx context.Context, templateID int64, fromDate time.Time) (*taskdomain.Task, error)
	Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]taskdomain.Task, error)
}

type Usecase interface {
	Create(ctx context.Context, input CreateInput) (*taskdomain.Task, error)
	GetByID(ctx context.Context, id int64) (*taskdomain.Task, error)
	Update(ctx context.Context, id int64, input UpdateInput) (*taskdomain.Task, error)
	Delete(ctx context.Context, id int64, scope taskdomain.DeleteScope) error
	List(ctx context.Context) ([]taskdomain.Task, error)
	Sync(ctx context.Context) error
}

type RecurrenceInput struct {
	EveryNDays  *int
	DayOfMonth  *int
	MonthsCount *int
	Dates       []string
	Parity      *string
}

type CreateInput struct {
	Title          string
	Description    string
	Status         taskdomain.Status
	RecurrenceKind taskdomain.RecurrenceKind
	Recurrence     RecurrenceInput
	StartDate      string
	AllDay         *bool
	StartTime      *string
	EndTime        *string
}

type UpdateInput struct {
	Title          string
	Description    string
	Status         taskdomain.Status
	RecurrenceKind *taskdomain.RecurrenceKind
	Recurrence     RecurrenceInput
	StartDate      *string
	AllDay         *bool
	StartTime      *string
	EndTime        *string
}
