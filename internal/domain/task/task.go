package task

import "time"

type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

type RecurrenceKind string

const (
	RecurrenceOneTime       RecurrenceKind = "one_time"
	RecurrenceDaily         RecurrenceKind = "daily"
	RecurrenceMonthlyDay    RecurrenceKind = "monthly_day"
	RecurrenceSpecificDate  RecurrenceKind = "specific_dates"
	RecurrenceMonthlyParity RecurrenceKind = "monthly_parity"
)

type MonthlyParity string

const (
	MonthlyParityOdd  MonthlyParity = "odd"
	MonthlyParityEven MonthlyParity = "even"
)

type Origin string

const (
	OriginManual    Origin = "manual"
	OriginGenerated Origin = "generated"
)

type RecurrenceRule struct {
	EveryNDays int           `json:"every_n_days,omitempty"`
	DayOfMonth int           `json:"day_of_month,omitempty"`
	Dates      []string      `json:"dates,omitempty"`
	Parity     MonthlyParity `json:"parity,omitempty"`
}

type Task struct {
	ID             int64          `json:"id"`
	TemplateID     int64          `json:"template_id"`
	TemplateStart  time.Time      `json:"-"`
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	Status         Status         `json:"status"`
	RecurrenceKind RecurrenceKind `json:"recurrence_kind"`
	Recurrence     RecurrenceRule `json:"recurrence"`
	ScheduledFor   time.Time      `json:"scheduled_for"`
	Origin         Origin         `json:"origin"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type Template struct {
	ID             int64
	Title          string
	Description    string
	DefaultStatus  Status
	RecurrenceKind RecurrenceKind
	Recurrence     RecurrenceRule
	StartDate      time.Time
	Active         bool
	GeneratedUntil time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusInProgress, StatusDone:
		return true
	default:
		return false
	}
}

func (k RecurrenceKind) Valid() bool {
	switch k {
	case RecurrenceOneTime, RecurrenceDaily, RecurrenceMonthlyDay, RecurrenceSpecificDate, RecurrenceMonthlyParity:
		return true
	default:
		return false
	}
}

func (p MonthlyParity) Valid() bool {
	switch p {
	case MonthlyParityOdd, MonthlyParityEven:
		return true
	default:
		return false
	}
}

func (o Origin) Valid() bool {
	switch o {
	case OriginManual, OriginGenerated:
		return true
	default:
		return false
	}
}
