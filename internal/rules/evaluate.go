package rules

import "fmt"

// Verdict is one row's judgement, for the renderer to place: the term it
// shows, and the row it came from.
type Verdict struct {
	Term string // a term id: the rule's verdict or otherwise
	Row  Row
	// Condition is true when the row's condition failed, so the row shows
	// its otherwise term with the note `condition`.
	Condition bool
}

// Facts is what an evaluation reads: the measure's value as Astrolog's
// output gave it, and category facts for row conditions.
type Facts interface {
	Value(key string) (float64, bool)
	Category(key string) (string, bool)
}

// Evaluate applies a within rule to the subject (a body's id): every row
// whose bodies include the subject (absent bodies: every body), in file
// order. It returns no verdicts when the rule omits itself because no row
// holds. A missing measure or condition fact is an error, never "not
// applicable": the caller decides absence before it asks.
//
// derivation: cited rule verdicts
func Evaluate(r Rule, subject string, facts Facts) ([]Verdict, error) {
	if r.Kind != "within" {
		return nil, fmt.Errorf("rules: %s: kind %q is not built", r.ID, r.Kind)
	}
	measure, ok := facts.Value(r.Measure)
	if !ok {
		return nil, fmt.Errorf("rules: %s: no value for its measure %s", r.ID, r.Measure)
	}
	var out []Verdict
	held := false
	for _, row := range r.Rows {
		if !appliesTo(row, subject) {
			continue
		}
		if row.Condition != nil {
			got, ok := facts.Category(row.Condition.Fact)
			if !ok {
				return nil, fmt.Errorf("rules: %s: no value for its condition %s", row.ID, row.Condition.Fact)
			}
			if got != row.Condition.Equals {
				out = append(out, Verdict{Term: r.Otherwise, Row: row, Condition: true})
				continue
			}
		}
		if within(measure, row) {
			held = true
			out = append(out, Verdict{Term: r.Verdict, Row: row})
		} else {
			out = append(out, Verdict{Term: r.Otherwise, Row: row})
		}
	}
	if !held && r.OmitWhenNoneHold {
		return nil, nil
	}
	return out, nil
}

// within is the row's test on the measure as parsed (never a display-rounded
// value): strictly below the threshold, or at it too when inclusive.
func within(measure float64, row Row) bool {
	if row.Inclusive {
		return measure <= row.Threshold
	}
	return measure < row.Threshold
}

func appliesTo(row Row, subject string) bool {
	if len(row.Bodies) == 0 {
		return true
	}
	for _, b := range row.Bodies {
		if b == subject {
			return true
		}
	}
	return false
}
