package keys

import (
	"slices"
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

func testRows() []Row {
	return []Row{
		{ID: BindingSlotMoveUp, Group: GroupLocal, Key: "K", Keys: []string{"K", "shift+up"}, Desc: "Move chart up"},
		{ID: BindingZodiac, Group: GroupGlobal, Key: "<ctrl+z>", Keys: []string{"ctrl+z"}, Desc: "Choose zodiac", Command: true},
		{ID: BindingOpenCharts, Group: GroupGlobal, Key: "<ctrl+o>", Keys: []string{"ctrl+o"}, Desc: "Open chart", Command: true},
		{ID: BindingTimeStepYear, Group: GroupNavigation, Key: "y/Y", Keys: []string{"y", "Y"}, Desc: "Step a year"},
	}
}

func descs(rows []Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Desc)
	}
	return out
}

func TestFilterRowsByDescriptionIgnoresCase(t *testing.T) {
	if got := descs(FilterRows(testRows(), "CHOOSE")); !slices.Equal(got, []string{"Choose zodiac"}) {
		t.Errorf("CHOOSE keeps %v", got)
	}
}

func TestFilterRowsByKeyIsExactAboutACapital(t *testing.T) {
	if got := descs(FilterRows(testRows(), "@K")); !slices.Equal(got, []string{"Move chart up"}) {
		t.Errorf("@K keeps %v", got)
	}
	if got := descs(FilterRows(testRows(), "@k")); !slices.Equal(got, []string{"Move chart up"}) {
		t.Errorf("@k keeps %v, want K found by its key name shift+up too", got)
	}
	if got := descs(FilterRows(testRows(), "@Y")); !slices.Equal(got, []string{"Step a year"}) {
		t.Errorf("@Y keeps %v", got)
	}
}

func TestFilterRowsCtrlSpellings(t *testing.T) {
	for _, q := range []string{"@<ctrl+z>", "@ctrl+z"} {
		if got := descs(FilterRows(testRows(), q)); !slices.Equal(got, []string{"Choose zodiac"}) {
			t.Errorf("%s keeps %v", q, got)
		}
	}
}

func TestLinesDrawOnlyGroupsThatMatchAndNeverStartOrEndOnABlankLine(t *testing.T) {
	st := NewKbState(testRows(), OwnerMain, nil)
	st.FilterText = "zod"
	st.Refilter()
	var kinds []LineKind
	for _, l := range st.Lines() {
		kinds = append(kinds, l.Kind)
	}
	if want := []LineKind{LineHeader, LineRow}; !slices.Equal(kinds, want) {
		t.Errorf("lines %v, want %v", kinds, want)
	}
}

func TestLinesPutOneBlankLineBetweenTwoGroupsAndNoSeparatorUnderRecentRows(t *testing.T) {
	rows := testRows()
	rows[1].Recent = true
	st := NewKbState(rows, OwnerMain, []string{"a note"})
	lines := st.Lines()
	if lines[0].Kind != LineHeader {
		t.Errorf("the list starts with kind %v, want a header", lines[0].Kind)
	}
	if last := lines[len(lines)-1]; last.Kind != LineNote {
		t.Errorf("the list ends with kind %v, want the note", last.Kind)
	}
	for i, l := range lines {
		if l.Kind != LineBlank {
			continue
		}
		if i == 0 || lines[i-1].Kind != LineRow || i+1 >= len(lines) || (lines[i+1].Kind != LineHeader && lines[i+1].Kind != LineNote) {
			t.Errorf("a blank line at %d is not between a group and the next group or note", i)
		}
	}
}

func TestKeybindingRowsAreOnceEachAndFaintWhereTheyCannotRun(t *testing.T) {
	cannot := func(Context) bool { return false }
	main := []BindingEntry{
		{ID: BindingSelect, Group: GroupLocal, Binding: key.NewBinding(key.WithKeys(" ")), Title: "Select", Help: true, When: cannot},
		{ID: BindingZodiac, Group: GroupGlobal, Binding: ctrlBinding(BindingZodiac), Title: "Choose zodiac", Help: true, Palette: true},
		{ID: BindingTimeStepYear, Group: GroupNavigation, Binding: key.NewBinding(key.WithKeys("y", "Y")), Title: "Step a year", Help: true},
		{ID: BindingTabNext, Group: GroupLocal, Binding: key.NewBinding(key.WithKeys("]"))},
	}
	main = Titled(main)
	in := KbInput{Origin: OwnerMain, OriginTable: main, Main: main, Recent: []BindingID{BindingZodiac}}
	rows := KeybindingRows(in)
	if got := descs(rows); !slices.Equal(got, []string{"Select", "Choose zodiac", "Step a year"}) {
		t.Fatalf("rows %v", got)
	}
	if !rows[0].Off || rows[1].Off || !rows[1].Recent || !rows[1].Command {
		t.Errorf("off/recent/command: %+v", rows)
	}
	in.Origin = OwnerHouses
	away := KeybindingRows(in)
	for _, r := range away {
		if r.Group != GroupLocal && !r.Off {
			t.Errorf("%q is not faint over a chooser", r.Desc)
		}
	}
}

func TestValidateReportsAnEntryWithNoGroup(t *testing.T) {
	r := Registry{Owners: map[Owner][][]BindingEntry{
		OwnerMain: {{{ID: BindingSelect, Binding: key.NewBinding(key.WithKeys("x")), Title: "Select"}}},
	}}
	got := Validate(r)
	if len(got) != 1 || got[0].Key != "Select" {
		t.Errorf("Validate = %v, want one clash naming Select", got)
	}
	r.Owners[OwnerMain][0][0].Group = GroupLocal
	if got := Validate(r); len(got) != 0 {
		t.Errorf("Validate = %v with the group declared", got)
	}
}
