package slices

import (
	"reflect"
	"strings"
	"testing"
)

const publisherPlan = `<!-- minion:research-slices parent=#42 -->

Slice plan for #42. Two slices, tracer-bullet ordered.

## Proposed slices

### Slice 1 — Plan parser

Parse the slice-plan comment into an ordered plan with strict validation.

#### Acceptance criteria

- [ ] Publisher format parses into an ordered plan
- [ ] Numbering gaps are rejected with a named error

#### Modules touched

- ` + "`slices`" + ` (new)
- ` + "`executor`" + `

#### Out of scope

- Resume-point computation — issue 03

### Slice 2 — Dry-run expansion

Expand the implement agent into one prompt per slice.

#### Acceptance criteria

- Prompt printed per slice
`

// handWrittenPlan is the same plan as publisherPlan typed by a human: no
// marker attributes, hyphen instead of em-dash, shallower heading levels,
// heading case/colon variance, star bullets, no checkboxes.
const handWrittenPlan = `<!-- minion:research-slices -->

## Slice 1 - Plan parser

Parse the slice-plan comment into an ordered plan with strict validation.

### Acceptance Criteria:

* Publisher format parses into an ordered plan
* Numbering gaps are rejected with a named error

### modules touched

* ` + "`slices`" + ` (new)
* ` + "`executor`" + `

### Out of scope:

* Resume-point computation — issue 03

## Slice 2 - Dry-run expansion

Expand the implement agent into one prompt per slice.

### Acceptance criteria

* Prompt printed per slice
`

var wantTwoSlices = []Slice{
	{
		Number:      1,
		Title:       "Plan parser",
		Description: "Parse the slice-plan comment into an ordered plan with strict validation.",
		AcceptanceCriteria: []string{
			"Publisher format parses into an ordered plan",
			"Numbering gaps are rejected with a named error",
		},
		ModulesTouched: []string{"`slices` (new)", "`executor`"},
		OutOfScope:     []string{"Resume-point computation — issue 03"},
	},
	{
		Number:             2,
		Title:              "Dry-run expansion",
		Description:        "Expand the implement agent into one prompt per slice.",
		AcceptanceCriteria: []string{"Prompt printed per slice"},
	},
}

func TestParse_PublisherFormat(t *testing.T) {
	plan, err := Parse(publisherPlan)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(plan.Slices, wantTwoSlices) {
		t.Errorf("Slices mismatch\ngot:  %#v\nwant: %#v", plan.Slices, wantTwoSlices)
	}
	if plan.Raw != publisherPlan {
		t.Errorf("Raw not preserved verbatim")
	}
}

func TestParse_Malformed(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing marker",
			body:    "### Slice 1 — Parser\n\nText.\n\n#### Acceptance criteria\n\n- One\n",
			wantErr: "missing <!-- minion:research-slices --> marker",
		},
		{
			name: "numbering gap",
			body: "<!-- minion:research-slices -->\n\n" +
				"### Slice 1 — Parser\n\n#### Acceptance criteria\n\n- One\n\n" +
				"### Slice 3 — Expansion\n\n#### Acceptance criteria\n\n- Two\n",
			wantErr: "expected slice 2, found slice 3",
		},
		{
			name: "empty acceptance criteria",
			body: "<!-- minion:research-slices -->\n\n" +
				"### Slice 1 — Parser\n\nText only, no criteria section.\n\n" +
				"### Slice 2 — Expansion\n\n#### Acceptance criteria\n\n- Two\n",
			wantErr: `slice 1 ("Parser"): no acceptance criteria`,
		},
		{
			name:    "marker but no slices",
			body:    "<!-- minion:research-slices -->\n\nJust prose, no slice headings.\n",
			wantErr: "no slices found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.body)
			if err == nil {
				t.Fatalf("Parse: want error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParse_HandWritten_ParsesIdentically(t *testing.T) {
	plan, err := Parse(handWrittenPlan)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(plan.Slices, wantTwoSlices) {
		t.Errorf("hand-written plan diverges from publisher parse\ngot:  %#v\nwant: %#v", plan.Slices, wantTwoSlices)
	}
}
