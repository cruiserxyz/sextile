package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// Each key that runs an action of its own is a row of its own in the
// Keybindings popup, with its own title and one key.

// splitKeys is every key the popup used to fold under a neighbour's row. Its
// action is in the table of the owner named, and each must be walked.
var splitKeys = []struct {
	table string
	id    keys.BindingID
}{
	{"main", keys.BindingSlotCycleDown}, {"main", keys.BindingPrevView},
	{"main", keys.BindingFocusChart}, {"main", keys.BindingFocusBodies}, {"main", keys.BindingFocusAspects},
	{"main", keys.BindingTabNext}, {"main", keys.BindingScrollDown}, {"main", keys.BindingPageDown},
	{"city", keys.BindingListDown}, {"city", keys.BindingPageDown},
	{"details", keys.BindingListDown}, {"details", keys.BindingTabPrev},
	{"details", keys.BindingFocusChart}, {"details", keys.BindingFocusBodies}, {"details", keys.BindingFocusAspects},
	{"details", keys.BindingScrollDown}, {"details", keys.BindingPageDown},
	{"console", keys.BindingListDown}, {"console", keys.BindingPageDown},
	{"experiments", keys.BindingListDown},
	{"log", keys.BindingListDown}, {"log", keys.BindingPageDown},
	{"save ask", keys.BindingChooseNext},
	{"new session ask", keys.BindingChooseNext},
	{"chart title", keys.BindingPrevField},
}

// splitTables are the tables the walk reads, by the names splitKeys uses.
func splitTables(m Model) map[string]struct {
	owner   owner
	entries []keys.BindingEntry
} {
	type tbl = struct {
		owner   owner
		entries []keys.BindingEntry
	}
	return map[string]tbl{
		"main":            {ownerMain, mainBindings(m.keys)},
		"city":            {ownerCity, cityBindings()},
		"details":         {ownerDetails, detailsBindings(m.keys)},
		"console":         {ownerConsole, consoleBindings()},
		"experiments":     {ownerExperiments, experimentsBindings()},
		"log":             {ownerLog, logBindings()},
		"save ask":        {ownerSaveChart, saveAskBindings()},
		"new session ask": {ownerSessions, newSessionAskBindings()},
		"timeline search": {ownerDetails, timelineSearchBindings()},
		"chart title":     {ownerChartTitle, chartTitleBindings()},
	}
}

// keyNames reads the key names out of the popup's spelling of a row's keys:
// `<ctrl+pgup>`, `[`, `<esc>/<left>/q`.
func keyNames(spelling string) []string {
	if spelling == "/" {
		return []string{"/"}
	}
	var out []string
	for _, part := range strings.Split(spelling, "/") {
		n := strings.TrimSuffix(strings.TrimPrefix(part, "<"), ">")
		if n == "space" {
			n = " "
		}
		out = append(out, n)
	}
	return out
}

// walkedRows is every row each table of the keyboard puts in the popup.
func walkedRows(m Model) (rows []keys.Row, tables map[string][]keys.Row) {
	tables = map[string][]keys.Row{}
	mainTable := mainBindings(m.keys)
	mainCtx := m.keysContextFor(ownerMain)
	for _, tb := range titleTables(m) {
		got := keys.KeybindingRows(keys.KbInput{
			Origin: tb.owner, OriginTable: tb.entries, OriginCtx: m.keysFacts(tb.owner),
			Main: mainTable, LeaderOnly: leaderOnlyBindings(), MainCtx: mainCtx,
		})
		rows = append(rows, got...)
		tables[tb.name] = got
	}
	return rows, tables
}

// With Details on screen the popup lists Scroll Details up and Scroll Details
// down as two rows, each with one key, and its key column is as wide as the
// longest single key.
func TestScrollDetailsHasARowForEachDirection(t *testing.T) {
	m := helpFixture(t, 144, 40)
	rows := map[string]keys.Row{}
	for _, r := range m.keybindingRows(ownerMain) {
		rows[r.Desc] = r
	}
	for desc, want := range map[string]string{
		"Scroll Details up": "<ctrl+pgup>", "Scroll Details down": "<ctrl+pgdown>",
	} {
		if r, ok := rows[desc]; !ok || r.Key != want {
			t.Errorf("row %q = %+v (found %v), want the key %q", desc, r, ok, want)
		}
	}
	st := keys.NewKbState(m.keybindingRows(ownerMain), ownerMain, nil)
	longest := 0
	for _, r := range st.All {
		for _, n := range keyNames(r.Key) {
			longest = max(longest, len([]rune(keys.Spell(n))))
		}
	}
	if st.KeyW > max(longest, keys.MinKeyW) {
		t.Errorf("the key column is %d wide, the longest single key is %d", st.KeyW, longest)
	}
}

// oppositeKeys are keys that run opposite actions: one steps or moves forward,
// the other back. An entry holds at most one of each pair, and a letter never
// stands with its own capital.
var oppositeKeys = [][2]string{
	{"up", "down"}, {"left", "right"}, {"pgup", "pgdown"}, {"ctrl+pgup", "ctrl+pgdown"},
	{"home", "end"}, {"tab", "shift+tab"}, {"[", "]"}, {"k", "j"}, {"K", "J"},
	{"shift+up", "shift+down"}, {"g", "G"}, {"h", "l"},
}

// No row of any table shows keys that run different actions. A key's action is
// the entry that matches it in its table (or the main table): every key a row
// spells resolves to that row, and no entry holds two keys that step or move
// in opposite directions, nor a letter with its capital.
func TestNoRowSpellsTheKeyOfAnotherAction(t *testing.T) {
	m := helpFixture(t, 144, 40)
	mainTable := mainBindings(m.keys)
	_, byTable := walkedRows(m)
	tables := titleTables(m)
	n := 0
	for _, tb := range tables {
		for _, e := range tb.entries {
			ks := e.Binding.Keys()
			for _, p := range oppositeKeys {
				if slices.Contains(ks, p[0]) && slices.Contains(ks, p[1]) {
					t.Errorf("%s: the entry %q holds %q and %q, which run opposite actions", tb.name, e.Title, p[0], p[1])
				}
			}
			for _, k := range ks {
				if up := strings.ToUpper(k); up != k && slices.Contains(ks, up) {
					t.Errorf("%s: the entry %q holds %q and %q, which run opposite actions", tb.name, e.Title, k, up)
				}
			}
		}
	}
	for _, tb := range tables {
		for _, r := range byTable[tb.name] {
			// The Copy and Console rows name the command's own key (`<ctrl+y>`, `, L l`),
			// which no table binding holds.
			if len(r.Keys) == 0 || r.Command || r.ID == keys.BindingCopy || r.ID == keys.BindingConsole {
				continue
			}
			n++
			for _, name := range keyNames(r.Key) {
				ownTable := keys.MatchBinding(pressedKey(name), tb.entries)
				if ownTable != r.ID && keys.MatchBinding(pressedKey(name), mainTable) != r.ID {
					t.Errorf("%s: row %q spells %q, which runs binding %d, not the row's %d", tb.name, r.Desc, name, int(ownTable), int(r.ID))
				}
			}
		}
	}
	if n < 100 {
		t.Fatalf("only %d rows walked", n)
	}
}

// Enter on a split row presses the row's own key, and that key is the row's
// action in its table. The walk is over every key the popup used to fold.
func TestEnterOnASplitRowRunsThatRowsAction(t *testing.T) {
	m := helpFixture(t, 144, 40)
	tables := splitTables(m)
	mainTable := mainBindings(m.keys)
	for _, s := range splitKeys {
		tb, ok := tables[s.table]
		if !ok {
			t.Fatalf("no table %q", s.table)
		}
		facts := m.keysFacts(tb.owner)
		var found bool
		for _, r := range keys.KeybindingRows(keys.KbInput{
			Origin: tb.owner, OriginTable: tb.entries, OriginCtx: facts,
			Main: mainTable, LeaderOnly: leaderOnlyBindings(), MainCtx: m.keysContextFor(ownerMain),
		}) {
			if r.ID != s.id {
				continue
			}
			found = true
			if r.Desc == "" || len(r.Keys) == 0 {
				t.Errorf("%s: id %d row %+v has no title or key", s.table, s.id, r)
				continue
			}
			if got := keys.MatchBinding(pressedKey(r.Keys[0]), tb.entries); got != s.id {
				t.Errorf("%s: Enter on %q presses %q, which runs binding %d, want %d", s.table, r.Desc, r.Keys[0], got, s.id)
			}
		}
		if !found {
			t.Errorf("%s: no popup row for binding %d", s.table, s.id)
		}
	}
}

// Enter on each Scroll Details row scrolls the report its own way.
func TestEnterOnScrollDetailsRowsScrollsBothWays(t *testing.T) {
	for _, c := range []struct {
		query string
		from  int
		want  int
	}{
		{"scroll details down", 0, detailsScrollStep},
		{"scroll details up", 2 * detailsScrollStep, detailsScrollStep},
	} {
		m := scrollModel(t, c.from)
		m.cfg = Config{Accessible: true, Color: ColorNever}
		m, _ = tpress(t, m, runes("?"))
		if !m.keybindingsOpen {
			t.Fatalf("%q: the popup did not open", c.query)
		}
		m = kbType(t, m, c.query)
		m, _ = tpress(t, m, keyEnter)
		if m.keybindingsOpen || m.details.scrollOffset != c.want {
			t.Errorf("%q: offset %d (popup open %v), want %d", c.query, m.details.scrollOffset, m.keybindingsOpen, c.want)
		}
	}
}

// Enter on each step row moves the floating time its own way, by its own unit.
func TestEnterOnAStepRowMovesTheTimeThatWay(t *testing.T) {
	steps := []struct {
		unit string
		move func(time.Time, int) time.Time
	}{
		{"a year", func(t time.Time, d int) time.Time { return t.AddDate(d, 0, 0) }},
		{"a month", func(t time.Time, d int) time.Time { return t.AddDate(0, d, 0) }},
		{"a week", func(t time.Time, d int) time.Time { return t.AddDate(0, 0, 7*d) }},
		{"a day", func(t time.Time, d int) time.Time { return t.AddDate(0, 0, d) }},
		{"an hour", func(t time.Time, d int) time.Time { return t.Add(time.Duration(d) * time.Hour) }},
		{"a minute", func(t time.Time, d int) time.Time { return t.Add(time.Duration(d) * time.Minute) }},
		{"a second", func(t time.Time, d int) time.Time { return t.Add(time.Duration(d) * time.Second) }},
	}
	for _, s := range steps {
		for _, dir := range []struct {
			word string
			d    int
		}{{"forward", +1}, {"back", -1}} {
			title := "step " + s.unit + " " + dir.word
			m := kbModel(t)
			berlin, err := time.LoadLocation("Europe/Berlin")
			if err != nil {
				t.Fatal(err)
			}
			before := m.targetFloatingTime.In(berlin)
			m, _ = tpress(t, m, runes("?"))
			if !m.keybindingsOpen {
				t.Fatalf("%q: the popup did not open", title)
			}
			m = kbType(t, m, title)
			if len(m.keybindings.Matches) != 1 {
				t.Fatalf("%q: %d rows match, want one", title, len(m.keybindings.Matches))
			}
			m, _ = tpress(t, m, keyEnter)
			if want := s.move(before, dir.d); m.keybindingsOpen || !m.targetFloatingTime.Equal(want) {
				t.Errorf("%q: the time is %v (popup open %v), want %v", title, m.targetFloatingTime, m.keybindingsOpen, want)
			}
		}
	}
}
