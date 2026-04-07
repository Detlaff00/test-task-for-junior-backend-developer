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
	day := 15
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
			input: RecurrenceInput{DayOfMonth: &day},
			assertion: func(t *testing.T, got taskdomain.RecurrenceRule) {
				if got.DayOfMonth != 15 {
					t.Fatalf("expected day_of_month=15, got %d", got.DayOfMonth)
				}
			},
		},
		{
			name:    "monthly day out of range",
			kind:    taskdomain.RecurrenceMonthlyDay,
			input:   RecurrenceInput{DayOfMonth: intPtr(31)},
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
		{
			name:  "one time ignores recurrence fields",
			kind:  taskdomain.RecurrenceOneTime,
			input: RecurrenceInput{DayOfMonth: &day},
			assertion: func(t *testing.T, got taskdomain.RecurrenceRule) {
				if !reflect.DeepEqual(got, taskdomain.RecurrenceRule{}) {
					t.Fatalf("expected empty rule, got %+v", got)
				}
			},
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

func TestBuildDueDates(t *testing.T) {
	t.Parallel()

	start := mustDate(t, "2026-04-01")
	from := mustDate(t, "2026-04-01")
	to := mustDate(t, "2026-04-10")

	tests := []struct {
		name     string
		template taskdomain.Template
		want     []string
	}{
		{
			name: "one time",
			template: taskdomain.Template{
				RecurrenceKind: taskdomain.RecurrenceOneTime,
				StartDate:      start,
			},
			want: []string{},
		},
		{
			name: "daily every 3",
			template: taskdomain.Template{
				RecurrenceKind: taskdomain.RecurrenceDaily,
				StartDate:      start,
				Recurrence:     taskdomain.RecurrenceRule{EveryNDays: 3},
			},
			want: []string{"2026-04-04", "2026-04-07", "2026-04-10"},
		},
		{
			name: "monthly day",
			template: taskdomain.Template{
				RecurrenceKind: taskdomain.RecurrenceMonthlyDay,
				StartDate:      start,
				Recurrence:     taskdomain.RecurrenceRule{DayOfMonth: 5},
			},
			want: []string{"2026-04-05"},
		},
		{
			name: "specific dates",
			template: taskdomain.Template{
				RecurrenceKind: taskdomain.RecurrenceSpecificDate,
				StartDate:      start,
				Recurrence:     taskdomain.RecurrenceRule{Dates: []string{"2026-04-03", "2026-04-09"}},
			},
			want: []string{"2026-04-03", "2026-04-09"},
		},
		{
			name: "odd days",
			template: taskdomain.Template{
				RecurrenceKind: taskdomain.RecurrenceMonthlyParity,
				StartDate:      start,
				Recurrence:     taskdomain.RecurrenceRule{Parity: taskdomain.MonthlyParityOdd},
			},
			want: []string{"2026-04-03", "2026-04-05", "2026-04-07", "2026-04-09"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildDueDates(tt.template, from, to)
			gotDates := make([]string, 0, len(got))
			for _, d := range got {
				gotDates = append(gotDates, d.Format(dateLayout))
			}
			if !reflect.DeepEqual(gotDates, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, gotDates)
			}
		})
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
