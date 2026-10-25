package ui

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The verifiers C16-C17: the three frame-producing computes and the Details
// compute carry their result's commands onto the message that commits them,
// and the overlay compute concatenates in completion order on every branch.

// frameStubs replaces the three lookup seams with canned results and records
// the order the computes called them in.
type frameStubs struct {
	natal, transit                 lookup.Result
	cross                          lookup.OverlayAspects
	natalErr, transitErr, crossErr error
	calls                          []string
}

func installFrameStubs(t *testing.T, s *frameStubs) {
	t.Helper()
	oldNatal, oldTransit, oldCross := firstCallNatalPositions, firstCallTransits, overlayCrossAspects
	t.Cleanup(func() {
		firstCallNatalPositions, firstCallTransits, overlayCrossAspects = oldNatal, oldTransit, oldCross
	})
	firstCallNatalPositions = func(context.Context, *lookup.Probers, store.AnchorChart, lookup.Lens) (lookup.Result, error) {
		s.calls = append(s.calls, "natal")
		return s.natal, s.natalErr
	}
	firstCallTransits = func(context.Context, *lookup.Probers, time.Time, location.PlaceInfo, lookup.Lens) (lookup.Result, error) {
		s.calls = append(s.calls, "transit")
		return s.transit, s.transitErr
	}
	overlayCrossAspects = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo) (lookup.OverlayAspects, error) {
		s.calls = append(s.calls, "cross")
		return s.cross, s.crossErr
	}
}

func installDetailsStubs(t *testing.T, single, cross func() (string, string, error)) {
	t.Helper()
	oldSingle, oldCross := computeRawReport, computeCrossBodyReport
	t.Cleanup(func() { computeRawReport, computeCrossBodyReport = oldSingle, oldCross })
	computeRawReport = func(context.Context, time.Time, location.PlaceInfo, lookup.Lens, lookup.RawReportRequest) (string, string, error) {
		return single()
	}
	computeCrossBodyReport = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo, aspect.Body) (string, string, error) {
		return cross()
	}
}

func commandsTestChart() store.AnchorChart {
	return store.AnchorChart{When: testFrozenTime().AddDate(-30, 0, 0), Place: testPlaceAmsterdam()}
}

// C16: every compute reaches lookup through a stubbable seam and carries the
// result's commands onto its message — on the error path too.
func TestFrameMessagesCarryCommands(t *testing.T) {
	natalCommands := []string{"natal positions", "natal aspects"}
	transitCommands := []string{"transit positions", "transit aspects"}
	boom := errors.New("boom")

	t.Run("computeChartCmd", func(t *testing.T) {
		installFrameStubs(t, &frameStubs{transit: lookup.Result{Commands: transitCommands}})
		msg := computeChartCmd(context.Background(), 1, 1, nil, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic)().(FirstFrameReadyMsg)
		if !slices.Equal(msg.Commands, transitCommands) {
			t.Errorf("Commands = %q, want %q", msg.Commands, transitCommands)
		}
	})
	t.Run("computeNatalSingleCmd", func(t *testing.T) {
		installFrameStubs(t, &frameStubs{natal: lookup.Result{Commands: natalCommands}})
		msg := computeNatalSingleCmd(context.Background(), 1, 1, nil, commandsTestChart(), lookup.Hellenistic)().(FirstFrameReadyMsg)
		if !slices.Equal(msg.Commands, natalCommands) {
			t.Errorf("Commands = %q, want %q", msg.Commands, natalCommands)
		}
	})
	t.Run("error frames keep the spawns that ran", func(t *testing.T) {
		installFrameStubs(t, &frameStubs{
			transit: lookup.Result{Commands: transitCommands[:1]}, transitErr: boom,
			natal: lookup.Result{Commands: natalCommands[:1]}, natalErr: boom,
		})
		chart := computeChartCmd(context.Background(), 1, 1, nil, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic)().(FirstFrameReadyMsg)
		if !errors.Is(chart.Err, boom) || !slices.Equal(chart.Commands, transitCommands[:1]) {
			t.Errorf("chart error frame: Err=%v Commands=%q", chart.Err, chart.Commands)
		}
		natal := computeNatalSingleCmd(context.Background(), 1, 1, nil, commandsTestChart(), lookup.Hellenistic)().(FirstFrameReadyMsg)
		if !errors.Is(natal.Err, boom) || !slices.Equal(natal.Commands, natalCommands[:1]) {
			t.Errorf("natal error frame: Err=%v Commands=%q", natal.Err, natal.Commands)
		}
	})
	t.Run("buildDetailsCmd single chart", func(t *testing.T) {
		installDetailsStubs(t,
			func() (string, string, error) { return "report", "details single", nil },
			func() (string, string, error) { t.Fatal("cross report must not run"); return "", "", nil },
		)
		m := detailsTestModel(t)
		m.focused = PanelChart
		msg := runDetailsCmd(t, m)
		p := msg.payload.(detailsPayload)
		if p.body != "report" || !slices.Equal(p.commands, []string{"details single"}) {
			t.Errorf("details payload = %+v, want the report and its command", p)
		}
	})
	t.Run("buildDetailsCmd comparison body", func(t *testing.T) {
		installDetailsStubs(t,
			func() (string, string, error) { t.Fatal("single report must not run"); return "", "", nil },
			func() (string, string, error) { return "cross report", "details cross", nil },
		)
		m := tabsOnPair(t, Config{Accessible: true})
		m.width, m.height = 144, 40
		m.focused = PanelBodies
		m.chart.positions = []aspect.Position{{Body: aspect.Moon}}
		p := runDetailsCmd(t, m).payload.(detailsPayload)
		if p.body != "cross report" || !slices.Equal(p.commands, []string{"details cross"}) {
			t.Errorf("details payload = %+v, want the cross report and its command", p)
		}
	})
	t.Run("buildDetailsCmd failure after spawn", func(t *testing.T) {
		installDetailsStubs(t,
			func() (string, string, error) { return "", "details failed", boom },
			func() (string, string, error) { return "", "", nil },
		)
		m := detailsTestModel(t)
		m.focused = PanelChart
		msg := runDetailsCmd(t, m)
		if !errors.Is(msg.err, boom) || !slices.Equal(msg.payload.(detailsPayload).commands, []string{"details failed"}) {
			t.Errorf("failed details: err=%v payload=%+v; want the command beside the error", msg.err, msg.payload)
		}
	})
	t.Run("buildDetailsCmd no spawn", func(t *testing.T) {
		installDetailsStubs(t,
			func() (string, string, error) { return "", "", boom },
			func() (string, string, error) { return "", "", nil },
		)
		m := detailsTestModel(t)
		m.focused = PanelChart
		if p := runDetailsCmd(t, m).payload.(detailsPayload); len(p.commands) != 0 {
			t.Errorf("a compute that never spawned carried %q", p.commands)
		}
	})
}

// C17: the overlay frame holds natal, transit and cross commands in the order
// the spawns completed, and each early return carries the steps that ran.
func TestOverlayFrameCarriesEveryCommand(t *testing.T) {
	boom := errors.New("boom")
	full := func() *frameStubs {
		return &frameStubs{
			natal:   lookup.Result{Commands: []string{"natal 1", "natal 2"}},
			transit: lookup.Result{Commands: []string{"transit 1", "transit 2"}},
			cross:   lookup.OverlayAspects{Commands: []string{"cross 1", "cross 2"}},
		}
	}
	cases := []struct {
		name      string
		arrange   func(s *frameStubs)
		wantCmds  []string
		wantCalls []string
		wantErr   error
	}{
		{"all three complete", func(*frameStubs) {},
			[]string{"natal 1", "natal 2", "transit 1", "transit 2", "cross 1", "cross 2"}, []string{"natal", "transit", "cross"}, nil},
		{"natal fails after one spawn", func(s *frameStubs) { s.natal = lookup.Result{Commands: []string{"natal 1"}}; s.natalErr = boom },
			[]string{"natal 1"}, []string{"natal"}, boom},
		{"transit fails after one spawn", func(s *frameStubs) { s.transit = lookup.Result{Commands: []string{"transit 1"}}; s.transitErr = boom },
			[]string{"natal 1", "natal 2", "transit 1"}, []string{"natal", "transit"}, boom},
		{"cross fails after the probe", func(s *frameStubs) { s.cross = lookup.OverlayAspects{Commands: []string{"cross 1"}}; s.crossErr = boom },
			[]string{"natal 1", "natal 2", "transit 1", "transit 2", "cross 1"}, []string{"natal", "transit", "cross"}, boom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := full()
			c.arrange(s)
			installFrameStubs(t, s)
			msg := computeOverlayCmd(context.Background(), 1, 1, nil, commandsTestChart(), store.AnchorChart{}, testFrozenTime(), testPlaceBerlin(), false, lookup.Hellenistic)().(FirstFrameReadyMsg)
			if !msg.Overlay {
				t.Error("not an overlay frame")
			}
			if !errors.Is(msg.Err, c.wantErr) {
				t.Errorf("Err = %v, want %v", msg.Err, c.wantErr)
			}
			if !slices.Equal(msg.Commands, c.wantCmds) {
				t.Errorf("Commands = %q, want %q", msg.Commands, c.wantCmds)
			}
			if !slices.Equal(s.calls, c.wantCalls) {
				t.Errorf("lookup calls = %q, want %q", s.calls, c.wantCalls)
			}
			// The message owns its list: it must not alias a result's backing
			// array, or a later append in one place would edit the other.
			if len(msg.Commands) > 0 {
				msg.Commands[0] = "edited"
				if s.natal.Commands[0] == "edited" {
					t.Error("the frame's Commands alias the natal result's slice")
				}
			}
		})
	}
}

// requirement C18 ("while this slice is the head, a carried command reaches no
// rendered surface") retired with an earlier change, which is the slice that renders
// them: TestCommittedFrameLogsItsCommands and TestDetailsCommitLogsItsCommand
// in log_lanes_test.go now pin the opposite, and TestSixRowLogHidesArgv keeps
// the half of C18 that survives (the 6-row Log still shows no command).
