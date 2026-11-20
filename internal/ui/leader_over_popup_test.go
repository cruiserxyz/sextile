package ui

import (
	"fmt"
	"github.com/cruiserxyz/sextile/internal/store"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// The command line over a popup: the leader key opens it from every filter
// and never becomes filter text, it offers only what acts on that popup, and
// every other chord is dormant until the popup releases the keyboard.

// overCase is one owner of a filter: how to open it with `kep` typed, and the
// filter text it holds.
type overCase struct {
	name   string
	owner  owner
	open   func(*testing.T) Model
	filter func(Model) string
}

// offersCopy is whether the owner binds `y`: Details' search lines and the console.
func offersCopy(name string) bool { return strings.HasSuffix(name, "search") || name == "console" }

func typedInto(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m, _ = tpress(t, m, runes(string(r)))
	}
	return m
}

func asModel(t *testing.T, tm any) Model {
	t.Helper()
	m, ok := tm.(Model)
	if !ok {
		t.Fatalf("not a Model: %T", tm)
	}
	return m
}

// overCases are the owners of a filter. Each name is what overOwnersOfTypeKey
// maps a caller of the shared filter key function to.
func overCases() []overCase {
	opener := func(owner owner, open func(Model) (any, any)) func(*testing.T) Model {
		return func(t *testing.T) Model {
			t.Helper()
			m := kbModel(t)
			m.startup = startupReady
			tm, _ := open(m)
			m = asModel(t, tm)
			if m.currentOwner() != owner {
				t.Fatalf("opened %v, want %v", m.currentOwner(), owner)
			}
			return typedInto(t, m, "kep")
		}
	}
	return []overCase{
		{"city", ownerCity, opener(ownerCity, func(m Model) (any, any) { return m.openCityPicker() }), func(m Model) string { return m.atlas.FilterText }},
		{"open chart", ownerOpenChart, opener(ownerOpenChart, func(m Model) (any, any) { return m.openChartPicker() }), func(m Model) string { return m.picker.FilterText }},
		{"chart list with a file", ownerOpenChart, func(t *testing.T) Model {
			m := kbModel(t)
			m.startup = startupReady
			tm, _ := m.openChartPicker()
			m = asModel(t, tm)
			m.picker.loaded = true
			m.picker.charts = []store.NamedChart{{ID: "kepler", Name: "Kepler", Source: store.ChartSource{Kind: store.ChartKindFile, File: "kepler.as"}}}
			m.picker.refilter()
			if !m.pickerRenames() {
				t.Fatal("fixture: Rename does not act on the row")
			}
			return m
		}, func(m Model) string { return m.picker.FilterText }},
		{"sessions", ownerSessions, func(t *testing.T) Model {
			m := sessionsModel(t, 1)
			otherSession(t, "Alpha", "Zed")
			m = openSessions(t, m)
			for i := 0; i < 6 && !m.sessionsOnFreeRow(); i++ {
				m, _ = press(t, m, "down")
			}
			if !m.sessionsOnFreeRow() || !m.sessionsCanTrash() {
				t.Fatal("fixture: the cursor is not on a session that can be renamed and deleted")
			}
			return m
		}, func(Model) string { return "" }},
		{"bodies", ownerBodies, opener(ownerBodies, func(m Model) (any, any) { return openBodySelector(m) }), func(m Model) string { return m.bodyModal.FilterText }},
		{"aspects", ownerAspects, opener(ownerAspects, func(m Model) (any, any) { return openAspectChooser(m) }), func(m Model) string { return m.aspectModal.FilterText }},
		{"houses", ownerHouses, opener(ownerHouses, func(m Model) (any, any) { return openHouseFinder(m) }), func(m Model) string { return m.houseFinder.FilterText }},
		{"zodiac", ownerZodiac, opener(ownerZodiac, func(m Model) (any, any) { return openZodiacFinder(m) }), func(m Model) string { return m.zodiacFinder.FilterText }},
		{"nodes", ownerNodes, opener(ownerNodes, func(m Model) (any, any) { return openNodesFinder(m) }), func(m Model) string { return m.nodesFinder.FilterText }},
		{"keybindings", ownerKeybindings, func(t *testing.T) Model {
			m := openedKeybindings(kbModel(t))
			return typedInto(t, m, "kep")
		}, func(m Model) string { return m.keybindings.FilterText }},
		{"timeline search", ownerDetails, func(t *testing.T) Model {
			m := onList(t, listTimeline, Config{Accessible: true})
			m, _ = tpress(t, m, runes("/"))
			if !(m.list(m.shownList()).searching && m.timelineSearchLine()) {
				t.Fatal("the timeline search did not open")
			}
			// Nothing typed: a query that keeps no row would leave nothing to copy.
			return m
		}, func(m Model) string { return m.list(m.shownList()).query }},
		{"console", ownerConsole, func(t *testing.T) Model { return openedConsole(t, 30) }, func(Model) string { return "" }},
		{"report search", ownerDetails, func(t *testing.T) Model {
			m := attachedReport(t, false, "[0] Aspects", ownFilterBody(8))
			m, _ = tpress(t, m, runes("/"))
			if !m.detailsReportSearchOpen() {
				t.Fatal("the report search did not open")
			}
			return typedInto(t, m, "kep")
		}, func(m Model) string { return m.details.query }},
	}
}

// overOwnersOfTypeKey maps each file that calls the shared filter key function
// to the cases above that it serves.
var overOwnersOfTypeKey = map[string][]string{
	"popup/atlas.go":             {"city"},
	"chart_modal.go":             {"open chart"},
	"popup/body.go":              {"bodies"},
	"popup/aspects.go":           {"aspects"},
	"popup/finder.go":            {"houses", "zodiac", "nodes"},
	"keybindings_popup_model.go": {"keybindings"},
	"search_line.go":             {"timeline search", "report search"},
}

// snapshot is what a chord must leave as it was.
func overSnapshot(m Model, c overCase) string {
	return fmt.Sprintf("%v|%v|%v|%v|%v|%v|%v|%v|%d|%d|%q|%v", m.startup, m.currentOwner(), m.keybindingsOpen, m.focused,
		m.logFocus.active, m.console.open, m.details.attached, m.leader.pending, len(m.logLines), m.cursorIdx, c.filter(m), m.sessions.Asking)
}

// wantMenu is what the command line over this owner must name, written from the
// owner's own table and the leader map: the entries marked as chords, each with
// its chord's word, and the comma form of every Ctrl letter key that can act, with
// the entry's own word (lower case). The entries are read, not listed by hand.
func wantMenu(m Model, c overCase) []string {
	facts := m.keysFacts(c.owner)
	var out []string
	for _, e := range m.ownerBindings(c.owner) {
		if e.Chord && m.blocked(e.ID) == "" {
			for _, cmd := range keys.Commands() {
				if cmd.ID == e.ID && cmd.Then == "" {
					out = append(out, cmd.Letter+" "+cmd.Word)
				}
			}
		}
		if e.When != nil && !e.When(facts) {
			continue
		}
		for _, k := range e.Binding.Keys() {
			if len(k) == 6 && strings.HasPrefix(k, "ctrl+") && k[5] >= 'a' && k[5] <= 'z' && k != "ctrl+c" && k != "ctrl+h" && k != "ctrl+i" && k != "ctrl+j" && k != "ctrl+m" {
				out = append(out, k[5:]+" "+strings.ToLower(e.TitleFor(facts)))
			}
		}
	}
	slices.Sort(out)
	return out
}

func barOf(m Model) string {
	return strings.TrimSpace(ansi.Strip(keys.RenderBar(m.keysContext(), 140)))
}

// Over a list with a filter, and over Keybindings, the leader changes nothing:
// no command line, no footer change, the popup and its filter as they were.
func TestLeaderOverAListDoesNothing(t *testing.T) {
	for _, c := range overCases() {
		t.Run(c.name, func(t *testing.T) {
			m := c.open(t)
			if len(wantMenu(m, c)) > 0 {
				t.Skip("this owner binds a chord")
			}
			before, bar := overSnapshot(m, c), barOf(m)
			after, _ := tpress(t, m, runes(","))
			if got := overSnapshot(after, c); got != before {
				t.Errorf("the leader changed something:\n%s\n%s", before, got)
			}
			if after.leader.pending || barOf(after) != bar {
				t.Errorf("the footer changed: %q to %q (pending %v)", bar, barOf(after), after.leader.pending)
			}
		})
	}
}

// Over a Details search line the leader opens the command line, which lists `y`
// and Esc, and `y` copies Details' own content.
func TestLeaderOverASearchLineListsCopy(t *testing.T) {
	for _, c := range overCases() {
		if !offersCopy(c.name) {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			copied := captureYank(t)
			m := c.open(t)
			m, _ = tpress(t, m, runes(","))
			typed := c.filter(m)
			if !m.leader.pending {
				t.Fatalf("pending %v, want the command line", m.leader.pending)
			}
			got := strings.Split(barOf(m), "  ")
			slices.Sort(got)
			if want := []string{"<esc>", "y copy"}; !slices.Equal(got, want) {
				t.Errorf("footer entries %q, want %q", got, want)
			}
			m, _ = tpress(t, m, runes("y"))
			if m.leader.pending || len(*copied) != 1 || c.filter(m) != typed {
				t.Errorf("y: pending %v, %d copies, filter %q, want %q", m.leader.pending, len(*copied), c.filter(m), typed)
			}
		})
	}
}

// What an owner offers is exactly what its table binds: the menu names the chords
// it marks and the comma forms of its Ctrl keys, and each of them runs.
func TestOfferedChordsAreTheBoundOnes(t *testing.T) {
	for _, c := range overCases() {
		t.Run(c.name, func(t *testing.T) {
			copied := captureYank(t)
			m := c.open(t)
			want := wantMenu(m, c)
			var got []string
			if offered := m.commandsOver(c.owner); true {
				for _, e := range m.keysFacts(c.owner).Leader.Twins {
					got = append(got, e.Letter+" "+e.Word)
				}
				for _, id := range offered {
					for _, cmd := range keys.Commands() {
						if cmd.ID == id && cmd.Then == "" {
							got = append(got, cmd.Letter+" "+cmd.Word)
						}
					}
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Errorf("offered %v, want what the table binds %v", got, want)
			}
			// What the menu lists is what the bar shows after the leader.
			if len(want) > 0 {
				mm, _ := tpress(t, m, runes(","))
				shown := strings.Split(barOf(mm), "  ")
				slices.Sort(shown)
				wantBar := append(slices.Clone(want), "<esc>")
				slices.Sort(wantBar)
				if !slices.Equal(shown, wantBar) {
					t.Errorf("the menu reads %q, want %q", shown, wantBar)
				}
			}
			// Each one runs.
			for _, w := range want {
				letter := strings.Fields(w)[0]
				before := overSnapshot(m, c)
				mm, _ := tpress(t, m, runes(","))
				mm, _ = tpress(t, mm, runes(letter))
				if overSnapshot(mm, c) == before && len(*copied) == 0 {
					t.Errorf("%q is listed and does nothing", w)
				}
			}
		})
	}
}

// Over each filter owner, every chord of the main command line (read from the
// binding table) that is not bound changes nothing.
func TestChordsAreDormantOverAPopup(t *testing.T) {
	chords := kbModel(t).leaderBindings
	if len(chords) < 10 {
		t.Fatalf("only %d chords read", len(chords))
	}
	for _, c := range overCases() {
		// Over an owner that offers nothing the leader itself does nothing, so the keys
		// after it are plain text (TestLeaderOverAListDoesNothing); the chords are
		// walked where the command line opens.
		if len(wantMenu(c.open(t), c)) == 0 {
			continue
		}
		for _, ch := range chords {
			t.Run(c.name+" "+keys.ShowSeq(ch.Seq), func(t *testing.T) {
				m := c.open(t)
				// A chord the owner lists runs; the others change nothing.
				for _, w := range wantMenu(m, c) {
					if strings.HasPrefix(w, ch.Seq[0]+" ") && len(ch.Seq) == 1 {
						return
					}
				}
				before := overSnapshot(m, c)
				m, _ = tpress(t, m, runes(","))
				for _, k := range ch.Seq {
					m, _ = tpress(t, m, runes(k))
				}
				if after := overSnapshot(m, c); after != before {
					t.Errorf("the chord changed something:\n%s\n%s", before, after)
				}
			})
		}
	}
}

// A comma never reaches a filter's text, whatever the leader is, and the
// leader never does either.
func TestACommaIsNeverFilterText(t *testing.T) {
	for _, c := range overCases() {
		t.Run(c.name, func(t *testing.T) {
			m := c.open(t)
			typed := c.filter(m)
			m.leaderKey = "x"
			m, _ = tpress(t, m, runes(","))
			if strings.Contains(c.filter(m), ",") || c.filter(m) != typed {
				t.Errorf("a comma changed the filter to %q", c.filter(m))
			}
			m, _ = tpress(t, m, runes("x"))
			if c.filter(m) != typed {
				t.Errorf("the leader x reached the filter: %q", c.filter(m))
			}
		})
	}
}

// Every caller of the shared filter key function, read from the code, is a
// file this walk knows, and every owner it names is walked above.
func TestEveryFilterOwnerIsWalked(t *testing.T) {
	walked := map[string]bool{}
	for _, c := range overCases() {
		walked[c.name] = true
	}
	callers := map[string]bool{}
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		f, perr := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "TypeKey" {
					callers[filepath.ToSlash(p)] = true
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(callers) < 6 {
		t.Fatalf("only %d callers of TypeKey read: %v", len(callers), callers)
	}
	for p := range callers {
		names, ok := overOwnersOfTypeKey[p]
		if !ok {
			t.Errorf("%s calls TypeKey and is not in the walk: add its owner to overCases and overOwnersOfTypeKey", p)
		}
		for _, n := range names {
			if !walked[n] {
				t.Errorf("%s serves %q, which overCases does not walk", p, n)
			}
		}
	}
	for p := range overOwnersOfTypeKey {
		if !callers[p] {
			t.Errorf("%s is in the walk but calls no TypeKey", p)
		}
	}
}
