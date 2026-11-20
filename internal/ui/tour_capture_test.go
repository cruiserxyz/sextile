package ui

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/tour"
)

// the Sextile Tour.
//
// docs/tour.svg is an animation of the real program, drawn by the capture lane
// from a key file: internal/ui/testdata/tours/sextile-tour.keys. The lane opens
// the tour chart from a charts folder of its own, starts the session the file
// names, and presses each key through Model.Update, the path a pressed key
// takes, with a fixed clock at 132 by 40. Each key's line says how long the
// picture rests on what it made. The picture is regenerated, never edited:
//
//	go test ./internal/ui -run '^TestTourIsDrawn$' -demo
//
// A key file made by `go run ./tools/tour record FILE` plays here without an edit.

const (
	tourKeysPath = "testdata/tours/sextile-tour.keys"
	tourPath     = "../../docs/tour.svg"
	tourCols     = 132
	tourRows     = 40

	// tourCut is the column where Details begins at this width. The animation
	// draws each half of a row on its own, so a half that did not change between
	// two keys is written once.
	tourCut = 65

	// The alt text is the SVG's <title> and, word for word, the README's
	// image description.
	tourAlt = "The Sextile Tour. The cursor goes from a chart to Now, through the planets, and into the aspects."
)

// tourNow is the frozen clock of every tour: the Now slot reads this moment.
var tourNow = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)

var tourAmsterdam = location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lat: 52.3667, Lon: 4.9, TZ: "Europe/Amsterdam"}

// tourFrame is the picture a step made and how long it rests. The first frame
// is the opening picture, before any key.
type tourFrame struct {
	Key   string
	Frame string
	Rest  time.Duration

	// What the model says, so a claim about focus is read from the program and
	// not guessed from the picture.
	Focused  PanelID
	Attached bool
	Tab      TabID
}

// playTour plays a key file through the real model with real Astrolog: the
// tour chart opened from a charts folder of its own, in a session of its own,
// then each key through Update.
func playTour(t *testing.T, script tour.Script) []tourFrame {
	t.Helper()
	needLiveAstrolog(t)
	forceColor(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	charts := filepath.Join(home, "charts")
	if err := os.MkdirAll(charts, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(charts, tour.ChartFile), tour.ChartText(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEXTILE_CHARTS_DIR", charts)
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")

	m := newTestModel(t, Config{Color: ColorAlways}, FrozenClock(tourNow))
	m.startup = startupReady
	m.place = tourAmsterdam
	slots, err := store.NewSlots(tourNow, tourAmsterdam)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.targetFloatingTime = slots.Floating().Time
	m.width, m.height = tourCols, tourRows
	m = pickLive(t, m, tour.ChartFile)
	if script.Session != "" {
		lock, err := store.SaveNamedSessionAs(script.Session, m.sessionSnapshot())
		if err != nil {
			t.Fatalf("the session %q was refused: %v", script.Session, err)
		}
		t.Cleanup(lock.Release)
		m.named = namedRun{file: lock.File(), name: script.Session, lock: lock}
	}

	if got, want := tour.Session(charts).Slots[0].Chart, m.sessionSnapshot().Slots[0].Chart; got.ID != want.ID || got.Name != want.Name ||
		!got.When.Equal(want.When) || got.Place != want.Place || got.Source != want.Source ||
		tour.Session(charts).Folder != m.sessionSnapshot().Folder {
		t.Fatalf("the session the tool starts a tour from is not what opening the chart saves:\n tool: %+v\n open: %+v", got, want)
	}

	see := func(key string, rest time.Duration) tourFrame {
		return tourFrame{Key: key, Frame: m.View(), Rest: rest,
			Focused: m.focused, Attached: m.details.attached, Tab: m.activeTabID(PanelDetails)}
	}
	frames := []tourFrame{see("", script.Start)}
	for _, st := range script.Steps {
		u, cmd := m.Update(st.Key)
		m = pumpPatient(t, u.(Model), cmd)
		frames = append(frames, see(tour.SpellKey(st.Key), st.Rest))
	}
	return frames
}

func readTour(t *testing.T, path string) tour.Script {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	script, err := tour.Parse(string(b))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return script
}

// tourAt is the frame the n-th press (from 1) of key made.
func tourAt(t *testing.T, frames []tourFrame, key string, n int) (int, tourFrame) {
	t.Helper()
	for i, f := range frames {
		if f.Key == key {
			if n--; n == 0 {
				return i, f
			}
		}
	}
	t.Fatalf("the tour has no press %d of %q", n, key)
	return 0, tourFrame{}
}

var (
	tourTimelineRowRE = regexp.MustCompile(`(?m)^[A-Z][a-z]{2} \d\d/\d\d/\d{4} +\d{1,2}:\d\d `)
	tourAspectRowRE   = regexp.MustCompile(`(?m)^ *\d+: `)
	tourCounterRE     = regexp.MustCompile(`(\d+) of (\d+)`)
)

// tourSide is one side of the frame as plain text: the panels (left) or
// Details (right), a line each.
func tourSide(frame string, right bool) []string {
	var out []string
	for _, line := range strings.Split(ansi.Strip(frame), "\n") {
		cells := []rune(line)
		if right {
			out = append(out, string(cells[min(tourCut, len(cells)):]))
		} else {
			out = append(out, string(cells[:min(tourCut, len(cells))]))
		}
	}
	return out
}

func tourDetails(frame string) string { return strings.Join(tourSide(frame, true), "\n") }

// detailsRows is the text of Details' rows: the border characters at its edges
// taken off.
func tourDetailsRows(frame string) []string {
	var out []string
	for _, l := range tourSide(frame, true) {
		l = strings.TrimRight(l, "│┃╯╮ ")
		l = strings.TrimPrefix(l, "│")
		out = append(out, strings.TrimLeft(l, "╭╰─"))
	}
	return out
}

func tourDetailsTitle(frame string) string {
	line := tourSide(frame, true)[0]
	title, _, _ := strings.Cut(strings.TrimPrefix(line, "╭─"), "─")
	return title
}

// tourBodiesCursor is the Bodies panel's counter: the row the cursor is on and
// how many rows there are.
func tourBodiesCursor(t *testing.T, frame string) (at, of int) {
	t.Helper()
	inBodies := false
	for _, l := range tourSide(frame, false) {
		switch {
		case strings.Contains(l, "[3] Bodies"):
			inBodies = true
		case inBodies && strings.HasPrefix(l, "╰"):
			if m := tourCounterRE.FindStringSubmatch(l); m != nil {
				at, _ = strconv.Atoi(m[1])
				of, _ = strconv.Atoi(m[2])
				return at, of
			}
		}
	}
	t.Fatalf("no Bodies counter in\n%s", ansi.Strip(frame))
	return 0, 0
}

// assertTourClaims holds the played frames to what the README's four numbered
// lines say, so the picture cannot show anything else.
func assertTourClaims(t *testing.T, frames []tourFrame) {
	t.Helper()
	plain := func(f tourFrame) string { return ansi.Strip(f.Frame) }

	open := frames[0]
	if p := plain(open); !strings.Contains(p, "Sextile Tour → ~/charts") ||
		!strings.Contains(p, "Sextile 0.1.0  Sun Oct 25, 2026 10:21:54 +1") ||
		!strings.Contains(p, "~ Now") || strings.ContainsAny(p, "①②") {
		t.Errorf("the opening picture is not the tour chart and Now in the session Sextile Tour, both unselected:\n%s", p)
	}
	if !strings.Contains(plain(open), "1 of 2") || open.Focused != PanelChart {
		t.Errorf("the opening focus is %v, want the first chart of two", open.Focused)
	}

	// 1. `↓` goes from the chart to Now; Enter goes into Details on the
	// Timeline, `]` shows the Moon, `]` again the report, and Esc comes back.
	_, down := tourAt(t, frames, "<down>", 1)
	if p := plain(down); !strings.Contains(p, "2 of 2") || down.Focused != PanelChart {
		t.Errorf("the first <down> did not put the cursor on Now:\n%s", p)
	}
	_, in := tourAt(t, frames, "<enter>", 1)
	if !in.Attached {
		t.Errorf("Enter on Now did not go into Details:\n%s", plain(in))
	}
	first := in
	_, second := tourAt(t, frames, "]", 1)
	_, third := tourAt(t, frames, "]", 2)
	_, out := tourAt(t, frames, "<esc>", 1)
	if !second.Attached || !third.Attached || out.Attached {
		t.Errorf("Details was entered %v, %v at the two ] and %v after Esc, want true, true, false", second.Attached, third.Attached, out.Attached)
	}
	if third.Tab == first.Tab || third.Tab == second.Tab {
		t.Errorf("the second ] shows tab %q again, want the third tab", third.Tab)
	}
	if first.Tab != TabDetailsTimeline || len(tourTimelineRowRE.FindAllString(strings.Join(tourDetailsRows(first.Frame), "\n"), -1)) < 10 {
		t.Errorf("Enter shows tab %q, want the Timeline with its month of rows:\n%s", first.Tab, tourDetails(first.Frame))
	}
	if second.Tab != TabDetailsMoon {
		t.Errorf("the first ] shows tab %q, want the Moon", second.Tab)
	}
	moonRows := 0
	for _, row := range tourDetailsRows(second.Frame) {
		if tourTimelineRowRE.MatchString(row) {
			moonRows++
			if !strings.Contains(row, "Moon") {
				t.Errorf("the Moon tab lists a row without the Moon: %q", row)
			}
		}
	}
	if moonRows < 10 {
		t.Errorf("the Moon tab lists %d rows, want at least 10", moonRows)
	}
	otherBody := false
	for _, row := range tourDetailsRows(first.Frame) {
		otherBody = otherBody || (tourTimelineRowRE.MatchString(row) && !strings.Contains(row, "Moon"))
	}
	if !otherBody {
		t.Error("the Timeline lists only Moon rows, so the first ] would show nothing new")
	}

	// 2. `3` goes to Bodies and Details shows the aspects of the planet under
	// the cursor; `↓` walks through all of them, down to the Midheaven.
	_, three := tourAt(t, frames, "3", 1)
	if at, of := tourBodiesCursor(t, three.Frame); three.Focused != PanelBodies || at != 1 || of < 5 {
		t.Errorf("3 left focus on %v with the Bodies cursor at %d of %d, want Bodies at the first row", three.Focused, at, of)
	}
	if title := tourDetailsTitle(three.Frame); title != "[0] Sun" || !strings.Contains(tourDetails(three.Frame), "Sun (") || !strings.Contains(tourDetails(three.Frame), "Sum power") {
		t.Errorf("Details after 3 is titled %q and lists %q, want the aspects of the Sun", title, tourDetails(three.Frame))
	}
	last := frames[0]
	prev := tourDetailsTitle(three.Frame)
	seen := map[string]bool{prev: true}
	walked := 0
	_, rows := tourBodiesCursor(t, three.Frame)
	for i, f := range frames {
		if f.Key != "<down>" || f.Focused != PanelBodies {
			continue
		}
		walked++
		last = f
		if at, _ := tourBodiesCursor(t, f.Frame); at != walked+1 {
			t.Errorf("step %d: the Bodies cursor is on row %d, want %d", i, at, walked+1)
		}
		title := tourDetailsTitle(f.Frame)
		if seen[title] {
			t.Errorf("step %d: Details is titled %q again; it should follow the cursor to a new body", i, title)
		}
		seen[title] = true
	}
	if at, of := tourBodiesCursor(t, last.Frame); walked != rows-1 || at != rows || of != rows {
		t.Errorf("the walk made %d steps and ends on row %d of %d, want %d steps to row %d", walked, at, of, rows-1, rows)
	}
	if tourDetailsTitle(last.Frame) != "[0] MC" {
		t.Errorf("the walk ends with Details titled %q, want the Midheaven", tourDetailsTitle(last.Frame))
	}
	if lines := tourSide(last.Frame, false); !bodiesLastRowIs(lines, "M MC") {
		t.Errorf("the last Bodies row is not the Midheaven:\n%s", strings.Join(lines, "\n"))
	}

	// 3. `4` goes to Aspects and Details lists every aspect.
	_, four := tourAt(t, frames, "4", 1)
	all := tourAspectRowRE.FindAllString(strings.Join(tourDetailsRows(four.Frame), "\n"), -1)
	if four.Focused != PanelAspects || tourDetailsTitle(four.Frame) != "[0] Aspects" || len(all) < 20 {
		t.Errorf("4 leaves focus on %v, Details titled %q with %d aspect rows", four.Focused, tourDetailsTitle(four.Frame), len(all))
	}

	// 4. Enter goes into Details; `/` and `ura` keep the rows with Uranus; Esc goes back.
	_, enter := tourAt(t, frames, "<enter>", 2)
	if !enter.Attached || !strings.Contains(plain(enter), "Back to Aspects: <esc>") {
		t.Errorf("Enter did not go into Details (attached %v):\n%s", enter.Attached, plain(enter))
	}
	prevCount := -1
	for _, key := range []string{"u", "r", "a"} {
		_, f := tourAt(t, frames, key, 1)
		m := tourCounterRE.FindStringSubmatch(strings.Join(tourDetailsRows(f.Frame), "\n"))
		if m == nil {
			t.Fatalf("no row count after %q in\n%s", key, tourDetails(f.Frame))
		}
		count, _ := strconv.Atoi(m[1])
		total, _ := strconv.Atoi(m[2])
		if count >= total || (prevCount >= 0 && count > prevCount) {
			t.Errorf("after %q the list holds %d of %d rows (before: %d), want it to shrink", key, count, total, prevCount)
		}
		prevCount = count
	}
	_, a := tourAt(t, frames, "a", 1)
	filtered := tourAspectRowRE.FindAllString(strings.Join(tourDetailsRows(a.Frame), "\n"), -1)
	if len(filtered) == 0 || len(filtered) != prevCount {
		t.Errorf("the filtered list shows %d rows, want the %d it counts", len(filtered), prevCount)
	}
	for _, row := range tourDetailsRows(a.Frame) {
		if tourAspectRowRE.MatchString(row) && !strings.Contains(row, "Uranus") {
			t.Errorf("the filtered list holds a row without Uranus: %q", row)
		}
	}
	_, slash := tourAt(t, frames, "/", 1)
	if !strings.Contains(strings.Join(tourDetailsRows(slash.Frame), "\n"), "/█") {
		t.Errorf("/ did not open the search line:\n%s", tourDetails(slash.Frame))
	}
	_, done := tourAt(t, frames, "<enter>", 3)
	if p := strings.Join(tourDetailsRows(done.Frame), "\n"); !strings.Contains(p, "/ura  ") || strings.Contains(p, "█") {
		t.Errorf("the second Enter did not finish the search with /ura kept:\n%s", tourDetails(done.Frame))
	}
	end := frames[len(frames)-1]
	if end.Key != "<esc>" || end.Focused != PanelAspects || strings.Contains(plain(end), "Back to Aspects") {
		t.Errorf("the tour ends on %q with focus on %v, want Esc with the focus back on Aspects", end.Key, end.Focused)
	}
	if !strings.Contains(strings.Join(tourDetailsRows(end.Frame), "\n"), "/ura  ") {
		t.Error("the filter is gone when Esc returns to Aspects")
	}
}

// bodiesLastRowIs is whether the last row of the Bodies list is the one that
// starts with label, in the left side's lines.
func bodiesLastRowIs(lines []string, label string) bool {
	last, inBodies := "", false
	for _, l := range lines {
		switch {
		case strings.Contains(l, "[3] Bodies"):
			inBodies = true
		case inBodies && strings.HasPrefix(l, "╰"):
			return strings.HasPrefix(last, label+" ")
		case inBodies && strings.HasPrefix(l, "│") && strings.TrimSpace(strings.Trim(l, "│")) != "":
			last = strings.TrimPrefix(l, "│")
		}
	}
	return false
}

// assertTourFrames holds every frame to the capture lane's rules for a frame.
func assertTourFrames(t *testing.T, frames []tourFrame) {
	t.Helper()
	for i, f := range frames {
		label := fmt.Sprintf("step %d (%s)", i, f.Key)
		assertFrameGeometry(t, f.Frame, tourCols, tourRows)
		assertSelfContainedFrame(t, label, f.Frame)
		assertAstrologColour(t, []demoFrame{{label, f.Frame}})
		if p := ansi.Strip(f.Frame); strings.Contains(p, "/tmp") || strings.Contains(p, "/home") || strings.Contains(p, os.TempDir()) {
			t.Errorf("%s carries a path of this machine:\n%s", label, p)
		}
		if top := []rune(strings.Split(ansi.Strip(f.Frame), "\n")[0]); top[tourCut-1] != '╮' || top[tourCut] != '╭' {
			t.Errorf("%s: Details does not begin at column %d", label, tourCut)
		}
	}
}

// TestTourIsDrawn plays the tour's own file on every run, holds each frame to
// the lane's rules and to the claims of the README, and writes docs/tour.svg
// only under -demo.
func TestTourIsDrawn(t *testing.T) {
	script := readTour(t, tourKeysPath)
	if script.Session != "Sextile Tour" {
		t.Fatalf("the tour's file names the session %q, want Sextile Tour", script.Session)
	}
	frames := playTour(t, script)
	assertTourFrames(t, frames)
	assertTourClaims(t, frames)
	svg := tourSVG(frames, tourAlt)
	if !*demoCapture {
		got, err := os.ReadFile(tourPath)
		if err != nil {
			t.Fatalf("docs/tour.svg is missing (regenerate with -demo, see tour_capture_test.go): %v", err)
		}
		if !bytes.Equal(got, svg) {
			t.Errorf("docs/tour.svg is not what the capture lane draws now (%d bytes committed, %d drawn): regenerate it with\n\tgo test ./internal/ui -run '^TestTourIsDrawn$' -demo", len(got), len(svg))
		}
		return
	}
	if err := os.WriteFile(tourPath, svg, 0o644); err != nil {
		t.Fatalf("write %s: %v", tourPath, err)
	}
	t.Logf("wrote %s (%d bytes, %d frames, %s)", filepath.Clean(tourPath), len(svg), len(frames), script.Total())
}

// cutSpans splits every span that crosses column cut, so no span stands on both
// sides of it.
func cutSpans(spans []svgSpan, cut int) []svgSpan {
	var out []svgSpan
	for _, sp := range spans {
		if sp.col >= cut || sp.col+sp.cells <= cut {
			out = append(out, sp)
			continue
		}
		left, right := sp, sp
		left.text, left.at, left.cells = "", nil, 0
		right.text, right.at, right.cells = "", nil, 0
		for k, r := range []rune(sp.text) {
			side := &left
			if sp.at[k] >= cut {
				side = &right
			}
			if len(side.at) == 0 {
				side.col = sp.at[k]
			}
			side.text += string(r)
			side.at = append(side.at, sp.at[k])
			side.cells += ansi.StringWidth(string(r))
		}
		out = append(out, left, right)
	}
	return out
}

// tourPiece is one half of one row, drawn: what it looks like and when it shows.
type tourPiece struct {
	row     int
	draw    string
	from    []time.Duration
	to      []time.Duration
	visible bool
}

// tourSVG draws the frames as one looping SVG: the first frame's size, and for
// each half-row every distinct picture it takes, shown for the spans of time
// the frames show it. A half-row that never changes is drawn once.
func tourSVG(frames []tourFrame, alt string) []byte {
	var rows, cols int
	pieces := map[string]*tourPiece{}
	var order []string
	var at, total time.Duration
	for _, f := range frames {
		total += f.Rest
	}
	for _, f := range frames {
		spans, r, c := svgSpans(strings.TrimRight(f.Frame, "\n"))
		rows, cols = r, c
		spans = cutSpans(spans, tourCut)
		type half struct{ row, side int }
		byHalf := map[half][]svgSpan{}
		for _, sp := range spans {
			side := 0
			if sp.col >= tourCut {
				side = 1
			}
			byHalf[half{sp.row, side}] = append(byHalf[half{sp.row, side}], sp)
		}
		keys := make([]half, 0, len(byHalf))
		for h := range byHalf {
			keys = append(keys, h)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].row != keys[j].row {
				return keys[i].row < keys[j].row
			}
			return keys[i].side < keys[j].side
		})
		for _, h := range keys {
			var bg, fg bytes.Buffer
			svgBackgrounds(&bg, byHalf[h])
			svgForeground(&fg, byHalf[h])
			if strings.TrimSpace(bg.String()+fg.String()) == "" {
				continue
			}
			id := fmt.Sprintf("%d/%d/%s\x00%s", h.row, h.side, bg.String(), fg.String())
			p := pieces[id]
			if p == nil {
				p = &tourPiece{row: h.row}
				if bg.Len() > 0 {
					p.draw += "<g shape-rendering=\"crispEdges\">\n" + bg.String() + "</g>\n"
				}
				p.draw += fg.String()
				pieces[id] = p
				order = append(order, id)
			}
			if n := len(p.to); n > 0 && p.to[n-1] == at {
				p.to[n-1] = at + f.Rest
			} else {
				p.from = append(p.from, at)
				p.to = append(p.to, at+f.Rest)
			}
		}
		at += f.Rest
	}

	var b bytes.Buffer
	esc := func(s string) string {
		var e bytes.Buffer
		_ = xml.EscapeText(&e, []byte(s))
		return e.String()
	}
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.1f %.1f" role="img" xml:space="preserve">`+"\n",
		float64(cols)*svgCellW, float64(rows)*svgCellH, float64(cols)*svgCellW, float64(rows)*svgCellH)
	fmt.Fprintf(&b, "<title>%s</title>\n", esc(alt))
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="%s"/>`+"\n", svgDefaultB)
	fmt.Fprintf(&b, `<g font-family="%s" font-size="%d">`+"\n", svgFontFam, svgFontSize)
	for _, id := range order {
		p := pieces[id]
		if len(p.from) == 1 && p.from[0] == 0 && p.to[0] == total {
			fmt.Fprintf(&b, "<g>\n%s</g>\n", p.draw)
			continue
		}
		fmt.Fprintf(&b, "<g visibility=\"hidden\">\n<animate attributeName=\"visibility\" calcMode=\"discrete\" dur=\"%s\" repeatCount=\"indefinite\" keyTimes=\"%s\" values=\"%s\"/>\n%s</g>\n",
			seconds(total), keyTimes(p, total), keyValues(p, total), p.draw)
	}
	fmt.Fprintf(&b, "</g>\n</svg>\n")
	return b.Bytes()
}

func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64) + "s"
}

// states is the piece's visibility at each moment it changes, from the start to the end.
func states(p *tourPiece, total time.Duration) (when []time.Duration, show []bool) {
	when, show = []time.Duration{0}, []bool{false}
	for k := range p.from {
		if p.from[k] == 0 {
			show[0] = true
		} else {
			when, show = append(when, p.from[k]), append(show, true)
		}
		if p.to[k] < total {
			when, show = append(when, p.to[k]), append(show, false)
		}
	}
	return when, show
}

func keyTimes(p *tourPiece, total time.Duration) string {
	when, _ := states(p, total)
	var out []string
	for _, w := range when {
		out = append(out, strconv.FormatFloat(float64(w)/float64(total), 'f', 5, 64))
	}
	return strings.Join(append(out, "1"), ";")
}

func keyValues(p *tourPiece, total time.Duration) string {
	_, show := states(p, total)
	var out []string
	for _, s := range show {
		out = append(out, map[bool]string{true: "visible", false: "hidden"}[s])
	}
	return strings.Join(append(out, out[len(out)-1]), ";")
}

// The picture is well-formed, carries the README's description as its title,
// moves, and holds nothing of the machine that made it.
func TestTourPictureIsWellFormed(t *testing.T) {
	b, err := os.ReadFile(tourPath)
	if err != nil {
		t.Fatalf("docs/tour.svg is missing (regenerate with -demo, see tour_capture_test.go): %v", err)
	}
	var title string
	animates := 0
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("docs/tour.svg is not well-formed XML: %v", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			switch se.Name.Local {
			case "title":
				if err := dec.DecodeElement(&title, &se); err != nil {
					t.Fatalf("reading <title>: %v", err)
				}
			case "animate":
				animates++
			}
		}
	}
	if title != tourAlt {
		t.Errorf("<title> = %q, want the alt text %q", title, tourAlt)
	}
	if animates == 0 {
		t.Error("docs/tour.svg does not move")
	}
	for _, banned := range []string{"/tmp", "/home"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("docs/tour.svg carries %q", banned)
		}
	}
	for _, want := range []string{"Sextile0.1.0", "Midheaven"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("docs/tour.svg does not show %q", want)
		}
	}
}

// The tour's own file says how a key file is written, in its own words.
func TestTourFileCarriesTheFormatAtItsTop(t *testing.T) {
	b, err := os.ReadFile(tourKeysPath)
	if err != nil {
		t.Fatal(err)
	}
	var head []string
	for _, l := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(l, "#") {
			break
		}
		head = append(head, strings.TrimPrefix(strings.TrimPrefix(l, "#"), " "))
	}
	if !strings.Contains(strings.Join(head, "\n"), tour.Format) {
		t.Errorf("the top of %s does not carry tour.Format:\n%s", tourKeysPath, strings.Join(head, "\n"))
	}
}

// A file written by `tour record` has no session line and starts where the
// opening picture rested. It plays here without an edit.
func TestARecordedFilePlaysInTheLaneWithoutAnEdit(t *testing.T) {
	recorded := tour.Script{Start: 1270 * time.Millisecond, Steps: []tour.Step{
		{Key: tea.KeyMsg{Type: tea.KeyDown}, Rest: 410 * time.Millisecond},
		{Key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}, Rest: 880 * time.Millisecond},
		{Key: tea.KeyMsg{Type: tea.KeyDown}, Rest: 90 * time.Millisecond},
	}}.Text()
	script, err := tour.Parse(recorded)
	if err != nil {
		t.Fatalf("what record writes does not parse: %v\n%s", err, recorded)
	}
	frames := playTour(t, script)
	if len(frames) != 4 || frames[0].Rest != 1270*time.Millisecond || frames[2].Rest != 880*time.Millisecond {
		t.Fatalf("played %d frames, the opening resting %s and the second step %s; want 4, 1.27s and 880ms", len(frames), frames[0].Rest, frames[2].Rest)
	}
	if p := ansi.Strip(frames[0].Frame); !strings.Contains(p, "unnamed →") {
		t.Errorf("a file with no session line runs unnamed:\n%s", p)
	}
	if last := frames[3]; last.Focused != PanelBodies || !strings.Contains(ansi.Strip(last.Frame), "2 of 18") {
		t.Errorf("after <down> 3 <down> the Bodies cursor is not on row 2 (focus %v)", last.Focused)
	}
}
