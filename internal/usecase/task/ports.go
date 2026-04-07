package task

import (
	"context"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type Repository interface {
	CreateTemplate(ctx context.Context, template *taskdomain.Template) (*taskdomain.Template, error)
	UpdateTemplate(ctx context.Context, template *taskdomain.Template) (*taskdomain.Template, error)
	ListTemplatesToGenerate(ctx context.Context, today time.Time) ([]taskdomain.Template, error)
	CreateInstances(ctx context.Context, template taskdomain.Template, dates []time.Time, origin taskdomain.Origin) error
	SetTemplateGeneratedUntil(ctx context.Context, templateID int64, generatedUntil time.Time) error

	GetByID(ctx context.Context, id int64) (*taskdomain.Task, error)
	Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]taskdomain.Task, error)
	GetLatestByTemplateID(ctx context.Context, templateID int64) (*taskdomain.Task, error)
}

type Usecase interface {
	Create(ctx context.Context, input CreateInput) (*taskdomain.Task, error)
	GetByID(ctx context.Context, id int64) (*taskdomain.Task, error)
	Update(ctx context.Context, id int64, input UpdateInput) (*taskdomain.Task, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]taskdomain.Task, error)
}

type RecurrenceInput struct {
	EveryNDays *int
	DayOfMonth *int
	Dates      []string
	Parity     *string
}

type CreateInput struct {
	Title          string
	Description    string
	Status         taskdomain.Status
	RecurrenceKind taskdomain.RecurrenceKind
	Recurrence     RecurrenceInput
	StartDate      string
}

type UpdateInput struct {
	Title          string
	Description    string
	Status         taskdomain.Status
	RecurrenceKind *taskdomain.RecurrenceKind
	Recurrence     RecurrenceInput
	StartDate      *string
}
