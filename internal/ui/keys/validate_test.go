package keys

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

func entry(id BindingID, ks ...string) BindingEntry {
	return BindingEntry{ID: id, Group: GroupLocal, Binding: key.NewBinding(key.WithKeys(ks...))}
}

func reg(o Owner, entries ...BindingEntry) Registry {
	return Registry{Owners: map[Owner][][]BindingEntry{o: {entries}}}
}

func wantClash(t *testing.T, cs []Clash, owner, k string) {
	t.Helper()
	for _, c := range cs {
		if c.Owner == owner && c.Key == k {
			if c.String() == "" {
				t.Error("empty String")
			}
			return
		}
	}
	t.Errorf("no clash for %s %s in %v", owner, k, cs)
}

func TestValidateReportsADuplicateKeyInOneOwner(t *testing.T) {
	cs := Validate(reg(OwnerCity, entry(BindingConfirm, "x"), entry(BindingClose, "x")))
	wantClash(t, cs, "city", "x")
}

func TestValidateReportsACtrlKeyTheTerminalOwns(t *testing.T) {
	cs := Validate(Registry{Commands: []Command{{Letter: "h", ID: BindingHouses}}})
	wantClash(t, cs, "commands", "ctrl+h")
}

func TestValidateReportsALowerCaseLetterWithNoCtrlForm(t *testing.T) {
	cs := Validate(Registry{Commands: []Command{{Letter: "c", ID: BindingCopy}}})
	wantClash(t, cs, "commands", "ctrl+c")
}

func TestValidateReportsACommandKeyTheMainScreenGaveAway(t *testing.T) {
	r := reg(OwnerMain, entry(BindingQuit, "ctrl+o"))
	r.Commands = []Command{{Letter: "o", ID: BindingOpenCharts}}
	wantClash(t, Validate(r), "commands", "ctrl+o")
}

func TestValidateReportsAPopupCtrlLetter(t *testing.T) {
	cs := Validate(reg(OwnerBodies, entry(BindingConfirm, "ctrl+e")))
	wantClash(t, cs, "bodies", "ctrl+e")
	if cs := Validate(reg(OwnerOpenChart, entry(BindingRename, "ctrl+r"), entry(BindingSessionDelete, "ctrl+d"))); len(cs) != 0 {
		t.Errorf("rename and delete are allowed: %v", cs)
	}
	if cs := Validate(reg(OwnerLog, entry(BindingConfirm, "ctrl+e"))); len(cs) != 0 {
		t.Errorf("the Log is not a popup: %v", cs)
	}
}

func TestValidateReportsTwoRowsWithOneSequence(t *testing.T) {
	cs := Validate(Registry{Commands: []Command{{Letter: "O", ID: BindingSessions}, {Letter: "O", ID: BindingLogConsole}}})
	wantClash(t, cs, "commands", "O")
	if !strings.Contains(cs[0].String(), "O") {
		t.Errorf("String %q", cs[0])
	}
}

func TestValidatePassesACapitalAndATwoKeyRow(t *testing.T) {
	cs := Validate(Registry{Commands: []Command{{Letter: "O", ID: BindingSessions}, {Letter: "c", Then: "d", ID: BindingSaveCity}}})
	if len(cs) != 0 {
		t.Errorf("clashes: %v", cs)
	}
}
