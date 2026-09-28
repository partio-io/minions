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

// consumerPlan places each identifier where exactly one lookup case needs
// it: ParseReport in slice 1 and, in backticks behind a selector, in slice
// 3; Load only in the heading of slice 2; Render only under a subsection
// Parse does not extract; Print only in the description of slice 3.
const consumerPlan = `<!-- minion:research-slices -->

### Slice 1 — Report parser

Add ParseReport to the report package.

#### Acceptance criteria

- ParseReport parses a report

### Slice 2 — Config Load

Add the config loader.

#### Acceptance criteria

- The loader reads the file

#### Notes

Render is added here for slice 3.

### Slice 3 — Wire the parser

Call ` + "`report.ParseReport()`" + ` from the command, then Print the result.

#### Acceptance criteria

- The command parses the report
`

func TestNamedAfter(t *testing.T) {
	plan, err := Parse(consumerPlan)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tests := []struct {
		name  string
		num   int
		ident string
		want  bool
	}{
		{name: "later slice names it in its description", num: 2, ident: "Print", want: true},
		{name: "later slice names it in a heading", num: 1, ident: "Load", want: true},
		{name: "later slice names it in backticks with a selector", num: 2, ident: "ParseReport", want: true},
		{name: "later slice names it under a subsection Parse ignores", num: 1, ident: "Render", want: true},
		{name: "only the current slice names it", num: 2, ident: "Load", want: false},
		{name: "only earlier slices name it", num: 3, ident: "Load", want: false},
		{name: "current and later slices name it", num: 1, ident: "ParseReport", want: true},
		{name: "no slice after the last", num: 3, ident: "ParseReport", want: false},
		{name: "prefix of a longer identifier is not a mention", num: 1, ident: "Parse", want: false},
		{name: "suffix of a longer identifier is not a mention", num: 1, ident: "Report", want: false},
		{name: "unknown identifier", num: 1, ident: "Missing", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plan.NamedAfter(tt.num, tt.ident); got != tt.want {
				t.Errorf("NamedAfter(%d, %q) = %v; want %v", tt.num, tt.ident, got, tt.want)
			}
		})
	}
	t.Run("nil plan names nothing", func(t *testing.T) {
		var nilPlan *Plan
		if nilPlan.NamedAfter(0, "ParseReport") {
			t.Error("NamedAfter on a nil plan = true; want false")
		}
	})
}
