package task

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

const dateLayout = "2006-01-02"

var weekdayParity = map[taskdomain.MonthlyParity]func(day int) bool{
	taskdomain.MonthlyParityOdd:  func(day int) bool { return day%2 == 1 },
	taskdomain.MonthlyParityEven: func(day int) bool { return day%2 == 0 },
}

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
	today := utcDate(s.now())
	normalized, err := validateCreateInput(input, today)
	if err != nil {
		return nil, err
	}

	now := s.now()
	tpl := &taskdomain.Template{
		Title:          normalized.Title,
		Description:    normalized.Description,
		DefaultStatus:  normalized.Status,
		RecurrenceKind: normalized.RecurrenceKind,
		Recurrence:     normalized.Recurrence,
		StartDate:      normalized.StartDate,
		AllDay:         normalized.AllDay,
		StartTime:      normalized.StartTime,
		EndTime:        normalized.EndTime,
		Active:         true,
		GeneratedUntil: today,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	createdTemplate, err := s.repo.CreateTemplate(ctx, tpl)
	if err != nil {
		return nil, err
	}

	dates := buildFutureDates(*createdTemplate, today)
	if len(dates) == 0 {
		return nil, fmt.Errorf("%w: no future dates to generate", ErrInvalidInput)
	}
	if err := s.repo.ReplaceFutureInstances(ctx, *createdTemplate, today, dates, taskdomain.OriginGenerated); err != nil {
		return nil, err
	}

	created, err := s.repo.GetNearestByTemplateID(ctx, createdTemplate.ID, today)
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

	today := utcDate(s.now())
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	normalized, err := validateUpdateInput(input)
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

	if !shouldUpdateTemplate(input) {
		return updated, nil
	}

	tpl, err := s.repo.GetTemplateByID(ctx, current.TemplateID)
	if err != nil {
		return nil, err
	}

	kind := tpl.RecurrenceKind
	if input.RecurrenceKind != nil {
		kind = *input.RecurrenceKind
	}
	if !kind.Valid() {
		return nil, fmt.Errorf("%w: invalid recurrence_kind", ErrInvalidInput)
	}

	recurrenceInput := input.Recurrence
	if isEmptyRecurrenceInput(recurrenceInput) {
		recurrenceInput = fromRule(tpl.Recurrence)
	}
	rule, err := normalizeRule(kind, recurrenceInput)
	if err != nil {
		return nil, err
	}

	startDate := tpl.StartDate
	if input.StartDate != nil {
		startDate, err = parseDate(*input.StartDate)
		if err != nil {
			return nil, err
		}
	}

	allDay := tpl.AllDay
	if input.AllDay != nil {
		allDay = *input.AllDay
	}

	startTime := tpl.StartTime
	if input.StartTime != nil {
		startTime = input.StartTime
	}
	endTime := tpl.EndTime
	if input.EndTime != nil {
		endTime = input.EndTime
	}
	allDay, startTime, endTime, err = normalizeTimeRange(allDay, startTime, endTime)
	if err != nil {
		return nil, err
	}

	tpl.Title = normalized.Title
	tpl.Description = normalized.Description
	tpl.DefaultStatus = normalized.Status
	tpl.RecurrenceKind = kind
	tpl.Recurrence = rule
	tpl.StartDate = startDate
	tpl.AllDay = allDay
	tpl.StartTime = startTime
	tpl.EndTime = endTime
	tpl.UpdatedAt = s.now()

	tpl, err = s.repo.UpdateTemplate(ctx, tpl)
	if err != nil {
		return nil, err
	}

	dates := buildFutureDates(*tpl, today)
	if err := s.repo.ReplaceFutureInstances(ctx, *tpl, today, dates, taskdomain.OriginGenerated); err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id int64, scope taskdomain.DeleteScope) error {
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	if scope == "" {
		scope = taskdomain.DeleteScopeSingle
	}
	if !scope.Valid() {
		return fmt.Errorf("%w: invalid delete scope", ErrInvalidInput)
	}

	if scope == taskdomain.DeleteScopeSingle {
		return s.repo.Delete(ctx, id)
	}

	today := utcDate(s.now())
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	tpl, err := s.repo.GetTemplateByID(ctx, task.TemplateID)
	if err != nil {
		return err
	}

	if err := s.repo.ReplaceFutureInstances(ctx, *tpl, today, nil, taskdomain.OriginGenerated); err != nil {
		return err
	}

	return s.repo.DeactivateTemplate(ctx, task.TemplateID)
}

func (s *Service) List(ctx context.Context) ([]taskdomain.Task, error) {
	return s.repo.List(ctx)
}

func (s *Service) Sync(ctx context.Context) error {
	today := utcDate(s.now())
	templates, err := s.repo.ListActiveTemplates(ctx)
	if err != nil {
		return err
	}

	for i := range templates {
		tpl := templates[i]
		dates := buildFutureDates(tpl, today)
		if err := s.repo.EnsureFutureInstances(ctx, tpl, today, dates, taskdomain.OriginGenerated); err != nil {
			return err
		}
	}

	return nil
}

type normalizedCreateInput struct {
	Title          string
	Description    string
	Status         taskdomain.Status
	RecurrenceKind taskdomain.RecurrenceKind
	Recurrence     taskdomain.RecurrenceRule
	StartDate      time.Time
	AllDay         bool
	StartTime      *string
	EndTime        *string
}

type normalizedUpdateInput struct {
	Title       string
	Description string
	Status      taskdomain.Status
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

	if kind == taskdomain.RecurrenceOneTime && startDate.Before(today) {
		return normalizedCreateInput{}, fmt.Errorf("%w: one_time start_date cannot be in the past", ErrInvalidInput)
	}

	allDay := true
	if input.AllDay != nil {
		allDay = *input.AllDay
	}
	allDay, startTime, endTime, err := normalizeTimeRange(allDay, input.StartTime, input.EndTime)
	if err != nil {
		return normalizedCreateInput{}, err
	}

	return normalizedCreateInput{
		Title:          title,
		Description:    description,
		Status:         status,
		RecurrenceKind: kind,
		Recurrence:     rule,
		StartDate:      startDate,
		AllDay:         allDay,
		StartTime:      startTime,
		EndTime:        endTime,
	}, nil
}

func validateUpdateInput(input UpdateInput) (normalizedUpdateInput, error) {
	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.Description)
	if title == "" {
		return normalizedUpdateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	if !input.Status.Valid() {
		return normalizedUpdateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	return normalizedUpdateInput{
		Title:       title,
		Description: description,
		Status:      input.Status,
	}, nil
}

func shouldUpdateTemplate(input UpdateInput) bool {
	return input.RecurrenceKind != nil || input.StartDate != nil || !isEmptyRecurrenceInput(input.Recurrence) || input.AllDay != nil || input.StartTime != nil || input.EndTime != nil
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
		if input.DayOfMonth == nil || *input.DayOfMonth < 1 || *input.DayOfMonth > 31 {
			return taskdomain.RecurrenceRule{}, fmt.Errorf("%w: monthly_day requires day_of_month in [1..31]", ErrInvalidInput)
		}
		if input.MonthsCount == nil || *input.MonthsCount < 1 || *input.MonthsCount > 12 {
			return taskdomain.RecurrenceRule{}, fmt.Errorf("%w: monthly_day requires months_count in [1..12]", ErrInvalidInput)
		}
		return taskdomain.RecurrenceRule{DayOfMonth: *input.DayOfMonth, MonthsCount: *input.MonthsCount}, nil
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

func buildFutureDates(template taskdomain.Template, today time.Time) []time.Time {
	start := utcDate(template.StartDate)
	from := maxDate(start, today)

	switch template.RecurrenceKind {
	case taskdomain.RecurrenceOneTime:
		if start.Before(today) {
			return nil
		}
		return []time.Time{start}
	case taskdomain.RecurrenceDaily:
		return generateDaily(start, from, start.AddDate(0, 2, 0), template.Recurrence.EveryNDays)
	case taskdomain.RecurrenceMonthlyDay:
		return generateMonthlyDay(start, from, template.Recurrence.DayOfMonth, template.Recurrence.MonthsCount)
	case taskdomain.RecurrenceSpecificDate:
		return generateSpecificDates(from, template.Recurrence.Dates)
	case taskdomain.RecurrenceMonthlyParity:
		return generateParity(from, start.AddDate(0, 1, 0), template.Recurrence.Parity)
	default:
		return nil
	}
}

func generateDaily(start, from, end time.Time, step int) []time.Time {
	if step <= 0 || end.Before(from) {
		return nil
	}
	cursor := start
	for cursor.Before(from) {
		cursor = cursor.AddDate(0, 0, step)
	}

	result := make([]time.Time, 0)
	for !cursor.After(end) {
		result = append(result, cursor)
		cursor = cursor.AddDate(0, 0, step)
	}
	return result
}

func generateMonthlyDay(start, from time.Time, day, monthsCount int) []time.Time {
	if day < 1 || day > 31 || monthsCount < 1 {
		return nil
	}
	result := make([]time.Time, 0, monthsCount)
	for i := 0; i < monthsCount; i++ {
		monthStart := time.Date(start.Year(), start.Month()+time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		lastDay := time.Date(monthStart.Year(), monthStart.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		targetDay := day
		if day > lastDay {
			targetDay = lastDay
		}
		candidate := time.Date(monthStart.Year(), monthStart.Month(), targetDay, 0, 0, 0, 0, time.UTC)
		if candidate.Before(start) || candidate.Before(from) {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func generateSpecificDates(from time.Time, dates []string) []time.Time {
	result := make([]time.Time, 0, len(dates))
	for _, raw := range dates {
		d, err := parseDate(raw)
		if err != nil {
			continue
		}
		if d.Before(from) {
			continue
		}
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Before(result[j]) })
	return result
}

func generateParity(from, endExclusive time.Time, parity taskdomain.MonthlyParity) []time.Time {
	predicate, ok := weekdayParity[parity]
	if !ok || !from.Before(endExclusive) {
		return nil
	}
	result := make([]time.Time, 0)
	for d := from; d.Before(endExclusive); d = d.AddDate(0, 0, 1) {
		if predicate(d.Day()) {
			result = append(result, d)
		}
	}
	return result
}

func normalizeTimeRange(allDay bool, startRaw, endRaw *string) (bool, *string, *string, error) {
	if allDay {
		return true, nil, nil, nil
	}
	start, err := normalizeClock(startRaw)
	if err != nil {
		return false, nil, nil, err
	}
	end, err := normalizeClock(endRaw)
	if err != nil {
		return false, nil, nil, err
	}
	if start == nil || end == nil {
		return false, nil, nil, fmt.Errorf("%w: start_time and end_time are required when all_day=false", ErrInvalidInput)
	}
	if minutes(*start) >= minutes(*end) {
		return false, nil, nil, fmt.Errorf("%w: start_time must be before end_time", ErrInvalidInput)
	}
	return false, start, end, nil
}

func normalizeClock(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	if _, err := time.Parse("15:04", value); err != nil {
		return nil, fmt.Errorf("%w: invalid time format, expected HH:MM", ErrInvalidInput)
	}
	return &value, nil
}

func minutes(value string) int {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	return h*60 + m
}

func isEmptyRecurrenceInput(input RecurrenceInput) bool {
	return input.EveryNDays == nil && input.DayOfMonth == nil && input.MonthsCount == nil && input.Parity == nil && len(input.Dates) == 0
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
	if rule.MonthsCount > 0 {
		v := rule.MonthsCount
		result.MonthsCount = &v
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

func maxDate(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
