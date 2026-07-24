package slices

import "testing"

func TestMarkerSubject(t *testing.T) {
	if got := MarkerSubject(3, 7); got != "minion:slice 3/7" {
		t.Errorf("MarkerSubject(3, 7) = %q; want %q", got, "minion:slice 3/7")
	}
}

func TestResumePoint(t *testing.T) {
	tests := []struct {
		name     string
		subjects []string
		total    int
		want     int
		wantErr  bool
	}{
		{
			name:     "no markers starts at slice one",
			subjects: []string{"initial"},
			total:    2,
			want:     0,
		},
		{
			name: "partial markers resume after the last completed slice",
			subjects: []string{
				"minion:slice 1/2",
				"slice 1/2: Plan parser",
				"initial",
			},
			total: 2,
			want:  1,
		},
		{
			name: "all markers leave nothing to build",
			subjects: []string{
				"minion:slice 2/2",
				"slice 2/2: Dry-run expansion",
				"minion:slice 1/2",
				"slice 1/2: Plan parser",
				"initial",
			},
			total: 2,
			want:  2,
		},
		{
			name: "markers exceeding the plan length fail loudly",
			subjects: []string{
				"minion:slice 3/3",
				"minion:slice 2/3",
				"minion:slice 1/3",
			},
			total:   2,
			wantErr: true,
		},
		{
			name: "work commits and marker-like prose are not markers",
			subjects: []string{
				"slice 1/2: work commit",
				"minion:slice with extra words",
				"feat: mention minion:slice 1/2 mid-subject",
			},
			total: 2,
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResumePoint(tt.subjects, tt.total)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResumePoint(%d markers, total %d) = %d, nil; want error", len(tt.subjects), tt.total, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResumePoint: %v", err)
			}
			if got != tt.want {
				t.Errorf("ResumePoint = %d; want %d", got, tt.want)
			}
		})
	}
}
