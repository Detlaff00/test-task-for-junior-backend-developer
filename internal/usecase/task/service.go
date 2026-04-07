package task

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

const dateLayout = "2006-01-02"

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*taskdomain.Task, error) {
	normalized, err := validateCreateInput(input, utcDate(s.now()))
	if err != nil {
		return nil, err
	}

	now := s.now()
	template := &taskdomain.Template{
		Title:          normalized.Title,
		Description:    normalized.Description,
		DefaultStatus:  normalized.Status,
		RecurrenceKind: normalized.RecurrenceKind,
		Recurrence:     normalized.Recurrence,
		StartDate:      normalized.StartDate,
		Active:         true,
		GeneratedUntil: normalized.StartDate.AddDate(0, 0, -1),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	createdTemplate, err := s.repo.CreateTemplate(ctx, template)
	if err != nil {
		return nil, err
	}

	if err := s.generateTemplateUntilToday(ctx, createdTemplate, taskdomain.OriginManual); err != nil {
		return nil, err
	}

	created, err := s.repo.GetLatestByTemplateID(ctx, createdTemplate.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: no task instance generated yet", ErrInvalidInput)
	}

	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) (*taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	normalized, err := validateUpdateInput(input, current, utcDate(s.now()))
	if err != nil {
		return nil, err
	}

	instance := &taskdomain.Task{
		ID:          id,
		Title:       normalized.Title,
		Description: normalized.Description,
		Status:      normalized.Status,
		UpdatedAt:   s.now(),
	}

	updated, err := s.repo.Update(ctx, instance)
	if err != nil {
		return nil, err
	}

	if normalized.TemplateUpdate == nil {
		return updated, nil
	}

	tpl := *normalized.TemplateUpdate
	tpl.ID = current.TemplateID
	tpl.UpdatedAt = s.now()

	updatedTemplate, err := s.repo.UpdateTemplate(ctx, &tpl)
	if err != nil {
		return nil, err
	}

	if err := s.generateTemplateUntilToday(ctx, updatedTemplate, taskdomain.OriginGenerated); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	return s.repo.Delete(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]taskdomain.Task, error) {
	if err := s.generateDueInstances(ctx); err != nil {
		return nil, err
	}

	return s.repo.List(ctx)
}

func (s *Service) generateDueInstances(ctx context.Context) error {
	today := utcDate(s.now())
	templates, err := s.repo.ListTemplatesToGenerate(ctx, today)
	if err != nil {
		return err
	}

	for i := range templates {
		tpl := templates[i]
		if err := s.generateTemplateUntilToday(ctx, &tpl, taskdomain.OriginGenerated); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) generateTemplateUntilToday(ctx context.Context, template *taskdomain.Template, origin taskdomain.Origin) error {
	today := utcDate(s.now())
	if !template.GeneratedUntil.Before(today) {
		return nil
	}

	dates := buildDueDates(*template, template.GeneratedUntil, today)
	if len(dates) > 0 {
		if err := s.repo.CreateInstances(ctx, *template, dates, origin); err != nil {
			return err
		}
	}

	if err := s.repo.SetTemplateGeneratedUntil(ctx, template.ID, today); err != nil {
		return err
	}
	template.GeneratedUntil = today

	return nil
}

type normalizedCreateInput struct {
	Title          string
	Description    string
	Status         taskdomain.Status
	RecurrenceKind taskdomain.RecurrenceKind
	Recurrence     taskdomain.RecurrenceRule
	StartDate      time.Time
}

type normalizedUpdateInput struct {
	Title          string
	Description    string
	Status         taskdomain.Status
	TemplateUpdate *taskdomain.Template
}

func validateCreateInput(input CreateInput, today time.Time) (normalizedCreateInput, error) {
	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.Description)
	if title == "" {
		return normalizedCreateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	status := input.Status
	if status == "" {
		status = taskdomain.StatusNew
	}
	if !status.Valid() {
		return normalizedCreateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	kind := input.RecurrenceKind
	if kind == "" {
		kind = taskdomain.RecurrenceOneTime
	}
	if !kind.Valid() {
		return normalizedCreateInput{}, fmt.Errorf("%w: invalid recurrence_kind", ErrInvalidInput)
	}

	rule, err := normalizeRule(kind, input.Recurrence)
	if err != nil {
		return normalizedCreateInput{}, err
	}

	startDate := today
	if strings.TrimSpace(input.StartDate) != "" {
		startDate, err = parseDate(input.StartDate)
		if err != nil {
			return normalizedCreateInput{}, err
		}
	}

	return normalizedCreateInput{
		Title:          title,
		Description:    description,
		Status:         status,
		RecurrenceKind: kind,
		Recurrence:     rule,
		StartDate:      startDate,
	}, nil
}

func validateUpdateInput(input UpdateInput, current *taskdomain.Task, today time.Time) (normalizedUpdateInput, error) {
	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.Description)
	if title == "" {
		return normalizedUpdateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	if !input.Status.Valid() {
		return normalizedUpdateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	result := normalizedUpdateInput{
		Title:       title,
		Description: description,
		Status:      input.Status,
	}

	if input.RecurrenceKind == nil && input.StartDate == nil && isEmptyRecurrenceInput(input.Recurrence) {
		return result, nil
	}

	kind := current.RecurrenceKind
	if input.RecurrenceKind != nil {
		kind = *input.RecurrenceKind
	}
	if !kind.Valid() {
		return normalizedUpdateInput{}, fmt.Errorf("%w: invalid recurrence_kind", ErrInvalidInput)
	}

	recurrenceInput := input.Recurrence
	if isEmptyRecurrenceInput(input.Recurrence) {
		recurrenceInput = fromRule(current.Recurrence)
	}

	rule, err := normalizeRule(kind, recurrenceInput)
	if err != nil {
		return normalizedUpdateInput{}, err
	}

	startDate := current.TemplateStart
	if input.StartDate != nil {
		startDate, err = parseDate(*input.StartDate)
		if err != nil {
			return normalizedUpdateInput{}, err
		}
	}

	generatedUntil := startDate.AddDate(0, 0, -1)
	if generatedUntil.Before(today) {
		generatedUntil = today.AddDate(0, 0, -1)
	}

	result.TemplateUpdate = &taskdomain.Template{
		Title:          title,
		Description:    description,
		DefaultStatus:  input.Status,
		RecurrenceKind: kind,
		Recurrence:     rule,
		StartDate:      startDate,
		Active:         true,
		GeneratedUntil: generatedUntil,
	}

	return result, nil
}

func normalizeRule(kind taskdomain.RecurrenceKind, input RecurrenceInput) (taskdomain.RecurrenceRule, error) {
	switch kind {
	case taskdomain.RecurrenceOneTime:
		return taskdomain.RecurrenceRule{}, nil
	case taskdomain.RecurrenceDaily:
		if input.EveryNDays == nil || *input.EveryNDays < 1 {
			return taskdomain.RecurrenceRule{}, fmt.Errorf("%w: daily requires every_n_days >= 1", ErrInvalidInput)
		}
		return taskdomain.RecurrenceRule{EveryNDays: *input.EveryNDays}, nil
	case taskdomain.RecurrenceMonthlyDay:
		if input.DayOfMonth == nil || *input.DayOfMonth < 1 || *input.DayOfMonth > 30 {
			return taskdomain.RecurrenceRule{}, fmt.Errorf("%w: monthly_day requires day_of_month in [1..30]", ErrInvalidInput)
		}
		return taskdomain.RecurrenceRule{DayOfMonth: *input.DayOfMonth}, nil
	case taskdomain.RecurrenceSpecificDate:
		if len(input.Dates) == 0 {
			return taskdomain.RecurrenceRule{}, fmt.Errorf("%w: specific_dates requires non-empty dates", ErrInvalidInput)
		}
		seen := make(map[string]struct{}, len(input.Dates))
		normalized := make([]string, 0, len(input.Dates))
		for _, raw := range input.Dates {
			date := strings.TrimSpace(raw)
			if _, err := parseDate(date); err != nil {
				return taskdomain.RecurrenceRule{}, err
			}
			if _, ok := seen[date]; ok {
				return taskdomain.RecurrenceRule{}, fmt.Errorf("%w: specific_dates must be unique", ErrInvalidInput)
			}
			seen[date] = struct{}{}
			normalized = append(normalized, date)
		}
		sort.Strings(normalized)
		return taskdomain.RecurrenceRule{Dates: normalized}, nil
	case taskdomain.RecurrenceMonthlyParity:
		if input.Parity == nil {
			return taskdomain.RecurrenceRule{}, fmt.Errorf("%w: monthly_parity requires parity", ErrInvalidInput)
		}
		parity := taskdomain.MonthlyParity(strings.TrimSpace(*input.Parity))
		if !parity.Valid() {
			return taskdomain.RecurrenceRule{}, fmt.Errorf("%w: parity must be odd or even", ErrInvalidInput)
		}
		return taskdomain.RecurrenceRule{Parity: parity}, nil
	default:
		return taskdomain.RecurrenceRule{}, fmt.Errorf("%w: invalid recurrence_kind", ErrInvalidInput)
	}
}

func buildDueDates(template taskdomain.Template, fromDate, toDate time.Time) []time.Time {
	start := utcDate(template.StartDate)
	current := utcDate(fromDate).AddDate(0, 0, 1)
	end := utcDate(toDate)
	if current.After(end) {
		return nil
	}

	result := make([]time.Time, 0)
	for !current.After(end) {
		if current.Before(start) {
			current = current.AddDate(0, 0, 1)
			continue
		}

		if matchesRule(template, current, start) {
			result = append(result, current)
		}

		current = current.AddDate(0, 0, 1)
	}

	return result
}

func matchesRule(template taskdomain.Template, date, start time.Time) bool {
	switch template.RecurrenceKind {
	case taskdomain.RecurrenceOneTime:
		return date.Equal(start)
	case taskdomain.RecurrenceDaily:
		if template.Recurrence.EveryNDays <= 0 {
			return false
		}
		diff := int(date.Sub(start).Hours() / 24)
		return diff%template.Recurrence.EveryNDays == 0
	case taskdomain.RecurrenceMonthlyDay:
		return date.Day() == template.Recurrence.DayOfMonth
	case taskdomain.RecurrenceSpecificDate:
		target := date.Format(dateLayout)
		for _, d := range template.Recurrence.Dates {
			if d == target {
				return true
			}
		}
		return false
	case taskdomain.RecurrenceMonthlyParity:
		if template.Recurrence.Parity == taskdomain.MonthlyParityOdd {
			return date.Day()%2 == 1
		}
		if template.Recurrence.Parity == taskdomain.MonthlyParityEven {
			return date.Day()%2 == 0
		}
		return false
	default:
		return false
	}
}

func isEmptyRecurrenceInput(input RecurrenceInput) bool {
	return input.EveryNDays == nil && input.DayOfMonth == nil && input.Parity == nil && len(input.Dates) == 0
}

func fromRule(rule taskdomain.RecurrenceRule) RecurrenceInput {
	result := RecurrenceInput{
		Dates: append([]string(nil), rule.Dates...),
	}
	if rule.EveryNDays > 0 {
		v := rule.EveryNDays
		result.EveryNDays = &v
	}
	if rule.DayOfMonth > 0 {
		v := rule.DayOfMonth
		result.DayOfMonth = &v
	}
	if rule.Parity != "" {
		v := string(rule.Parity)
		result.Parity = &v
	}
	return result
}

func parseDate(raw string) (time.Time, error) {
	date, err := time.ParseInLocation(dateLayout, strings.TrimSpace(raw), time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid date format, expected YYYY-MM-DD", ErrInvalidInput)
	}
	return utcDate(date), nil
}

func utcDate(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
