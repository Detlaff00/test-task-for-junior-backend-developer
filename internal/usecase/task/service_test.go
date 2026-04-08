package task

import (
	"errors"
	"reflect"
	"testing"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

func TestNormalizeRule(t *testing.T) {
	t.Parallel()

	every := 2
	day := 31
	months := 3
	odd := "odd"

	tests := []struct {
		name      string
		kind      taskdomain.RecurrenceKind
		input     RecurrenceInput
		wantErr   bool
		assertion func(t *testing.T, got taskdomain.RecurrenceRule)
	}{
		{
			name:  "daily valid",
			kind:  taskdomain.RecurrenceDaily,
			input: RecurrenceInput{EveryNDays: &every},
			assertion: func(t *testing.T, got taskdomain.RecurrenceRule) {
				if got.EveryNDays != 2 {
					t.Fatalf("expected every_n_days=2, got %d", got.EveryNDays)
				}
			},
		},
		{
			name:    "daily invalid",
			kind:    taskdomain.RecurrenceDaily,
			input:   RecurrenceInput{},
			wantErr: true,
		},
		{
			name:  "monthly day valid",
			kind:  taskdomain.RecurrenceMonthlyDay,
			input: RecurrenceInput{DayOfMonth: &day, MonthsCount: &months},
			assertion: func(t *testing.T, got taskdomain.RecurrenceRule) {
				if got.DayOfMonth != 31 || got.MonthsCount != 3 {
					t.Fatalf("expected monthly fields, got %+v", got)
				}
			},
		},
		{
			name:    "monthly day missing months_count",
			kind:    taskdomain.RecurrenceMonthlyDay,
			input:   RecurrenceInput{DayOfMonth: intPtr(10)},
			wantErr: true,
		},
		{
			name:  "specific dates valid and sorted",
			kind:  taskdomain.RecurrenceSpecificDate,
			input: RecurrenceInput{Dates: []string{"2026-05-10", "2026-05-01"}},
			assertion: func(t *testing.T, got taskdomain.RecurrenceRule) {
				want := []string{"2026-05-01", "2026-05-10"}
				if !reflect.DeepEqual(got.Dates, want) {
					t.Fatalf("expected %v, got %v", want, got.Dates)
				}
			},
		},
		{
			name:    "specific dates duplicate",
			kind:    taskdomain.RecurrenceSpecificDate,
			input:   RecurrenceInput{Dates: []string{"2026-05-01", "2026-05-01"}},
			wantErr: true,
		},
		{
			name:  "monthly parity valid",
			kind:  taskdomain.RecurrenceMonthlyParity,
			input: RecurrenceInput{Parity: &odd},
			assertion: func(t *testing.T, got taskdomain.RecurrenceRule) {
				if got.Parity != taskdomain.MonthlyParityOdd {
					t.Fatalf("expected odd, got %s", got.Parity)
				}
			},
		},
		{
			name:    "monthly parity invalid",
			kind:    taskdomain.RecurrenceMonthlyParity,
			input:   RecurrenceInput{Parity: strPtr("nope")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeRule(tt.kind, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("expected ErrInvalidInput, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.assertion != nil {
				tt.assertion(t, got)
			}
		})
	}
}

func TestBuildFutureDates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		today    string
		template taskdomain.Template
		want     []string
	}{
		{
			name:  "one time future",
			today: "2026-04-01",
			template: taskdomain.Template{
				RecurrenceKind: taskdomain.RecurrenceOneTime,
				StartDate:      mustDate(t, "2026-04-03"),
			},
			want: []string{"2026-04-03"},
		},
		{
			name:  "daily every 3 for 2 months",
			today: "2026-04-01",
			template: taskdomain.Template{
				RecurrenceKind: taskdomain.RecurrenceDaily,
				StartDate:      mustDate(t, "2026-04-01"),
				Recurrence:     taskdomain.RecurrenceRule{EveryNDays: 3},
			},
			want: []string{"2026-04-01", "2026-04-04", "2026-04-07", "2026-04-10"},
		},
		{
			name:  "monthly day fallback to last day",
			today: "2026-01-01",
			template: taskdomain.Template{
				RecurrenceKind: taskdomain.RecurrenceMonthlyDay,
				StartDate:      mustDate(t, "2026-01-31"),
				Recurrence:     taskdomain.RecurrenceRule{DayOfMonth: 31, MonthsCount: 3},
			},
			want: []string{"2026-01-31", "2026-02-28", "2026-03-31"},
		},
		{
			name:  "specific dates filters past",
			today: "2026-04-01",
			template: taskdomain.Template{
				RecurrenceKind: taskdomain.RecurrenceSpecificDate,
				StartDate:      mustDate(t, "2026-04-01"),
				Recurrence:     taskdomain.RecurrenceRule{Dates: []string{"2026-03-31", "2026-04-03", "2026-04-09"}},
			},
			want: []string{"2026-04-03", "2026-04-09"},
		},
		{
			name:  "odd days for one month window",
			today: "2026-04-01",
			template: taskdomain.Template{
				RecurrenceKind: taskdomain.RecurrenceMonthlyParity,
				StartDate:      mustDate(t, "2026-04-01"),
				Recurrence:     taskdomain.RecurrenceRule{Parity: taskdomain.MonthlyParityOdd},
			},
			want: []string{"2026-04-01", "2026-04-03", "2026-04-05", "2026-04-07", "2026-04-09"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			today := mustDate(t, tt.today)
			got := buildFutureDates(tt.template, today)
			gotDates := make([]string, 0, len(got))
			for _, d := range got {
				gotDates = append(gotDates, d.Format(dateLayout))
			}

			if len(tt.want) < len(gotDates) {
				gotDates = gotDates[:len(tt.want)]
			}
			if !reflect.DeepEqual(gotDates, tt.want) {
				t.Fatalf("expected prefix %v, got %v", tt.want, gotDates)
			}
		})
	}
}

func TestNormalizeTimeRange(t *testing.T) {
	t.Parallel()

	allDay := true
	if gotAllDay, start, end, err := normalizeTimeRange(allDay, strPtr("09:00"), strPtr("10:00")); err != nil || !gotAllDay || start != nil || end != nil {
		t.Fatalf("all day should clear times, got allDay=%v start=%v end=%v err=%v", gotAllDay, start, end, err)
	}

	if _, _, _, err := normalizeTimeRange(false, strPtr("10:00"), strPtr("09:00")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for inverted time range, got %v", err)
	}
}

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := parseDate(s)
	if err != nil {
		t.Fatalf("parse date %s: %v", s, err)
	}
	return d
}
