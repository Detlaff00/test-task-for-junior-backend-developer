package handlers

import (
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type recurrenceDTO struct {
	EveryNDays  *int     `json:"every_n_days,omitempty"`
	DayOfMonth  *int     `json:"day_of_month,omitempty"`
	MonthsCount *int     `json:"months_count,omitempty"`
	Dates       []string `json:"dates,omitempty"`
	Parity      *string  `json:"parity,omitempty"`
}

type taskMutationDTO struct {
	Title          string                    `json:"title"`
	Description    string                    `json:"description"`
	Status         taskdomain.Status         `json:"status"`
	RecurrenceKind taskdomain.RecurrenceKind `json:"recurrence_kind,omitempty"`
	Recurrence     recurrenceDTO             `json:"recurrence,omitempty"`
	StartDate      string                    `json:"start_date,omitempty"`
	AllDay         *bool                     `json:"all_day,omitempty"`
	StartTime      *string                   `json:"start_time,omitempty"`
	EndTime        *string                   `json:"end_time,omitempty"`
}

type taskDTO struct {
	ID             int64                     `json:"id"`
	TemplateID     int64                     `json:"template_id"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description"`
	Status         taskdomain.Status         `json:"status"`
	RecurrenceKind taskdomain.RecurrenceKind `json:"recurrence_kind"`
	Recurrence     taskdomain.RecurrenceRule `json:"recurrence"`
	ScheduledFor   string                    `json:"scheduled_for"`
	AllDay         bool                      `json:"all_day"`
	StartTime      *string                   `json:"start_time,omitempty"`
	EndTime        *string                   `json:"end_time,omitempty"`
	Origin         taskdomain.Origin         `json:"origin"`
	CreatedAt      time.Time                 `json:"created_at"`
	UpdatedAt      time.Time                 `json:"updated_at"`
}

func newTaskDTO(task *taskdomain.Task) taskDTO {
	return taskDTO{
		ID:             task.ID,
		TemplateID:     task.TemplateID,
		Title:          task.Title,
		Description:    task.Description,
		Status:         task.Status,
		RecurrenceKind: task.RecurrenceKind,
		Recurrence:     task.Recurrence,
		ScheduledFor:   task.ScheduledFor.UTC().Format("2006-01-02"),
		AllDay:         task.AllDay,
		StartTime:      task.StartTime,
		EndTime:        task.EndTime,
		Origin:         task.Origin,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}
}
