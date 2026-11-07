package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// actions.go is where a command runs by its id, and where the one reason it
// cannot run now is written. The main keys, the leader and (next) the Actions
// palette all come here, so an action is written once and asked the same
// question wherever it is started.
//
// runAction is the one switch of the commands: the 24 ids whose table entry
// carries Palette.
// Motion, paging, tab switching and the time steps are not commands and stay
// with the key that presses them (dispatchMainBinding).
//
// blocked is the other half: "" when the command can run, else the reason in
// the screen's own words. Every condition that stops one of the commands lives
// in blocked and nowhere else; the opener of each command asks it and does, on
// that path, exactly what it did before the question had a home. runAction does
// not ask it: the opener does.

// The reasons a command is blocked. Each starts lower case, is at most 32
// characters, and names what is missing the way the screen already does.
const (
	whyNoCity         = "needs a city with a time zone"
	whyLoading        = "the chart is still loading"
	whyNoChart        = "no chart under the cursor"
	whyOnlyNow        = "only Now can be saved"
	whyNoBack         = "no earlier time to go back to"
	whyNotOnOpened    = "focus a chart you opened"
	whyNoneAbove      = "no chart above this one"
	whyNoneBelow      = "no chart below this one"
	whyNoDetails      = "the Details panel is not shown"
	whyNothingToPick  = "nothing here to select"
	whyAspectsUnread  = "aspect list could not be read"
	whyBodiesUnread   = "body list could not be read"
	whyDrawnUnread    = "drawn bodies could not be read"
	whyNotFirstWindow = "this window keeps no session"
	whySessionEmpty   = "the session is already empty"
)

// ran is runAction's answer for an id it runs.
func ran(tm tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd, bool) { return tm, cmd, true }

// runAction runs the command named by id, and says whether it is one. An id
// that is not a command (a motion, a page, a tab, a time step) comes back
// unchanged with false.
func (m Model) runAction(id keys.BindingID) (tea.Model, tea.Cmd, bool) {
	switch id {
	case keys.BindingQuit:
		return ran(m.quit())
	case keys.BindingHelp:
		m.helpOpen = true
		m.help = keys.HelpState{}
		return ran(m, nil)
	case keys.BindingOpenAtlas:
		return ran(m.openCityPicker())
	case keys.BindingOpenCharts:
		return ran(m.openChartPicker())
	case keys.BindingSaveChart:
		return ran(m.openSavePrompt())
	case keys.BindingAspects:
		return ran(openAspectChooser(m))
	case keys.BindingBodies:
		return ran(openBodySelector(m))
	case keys.BindingHouses:
		return ran(openHouseFinder(m))
	case keys.BindingZodiac:
		return ran(openZodiacFinder(m))
	case keys.BindingAttach:
		return ran(m.attachDetails())
	case keys.BindingSelect:
		return ran(m.toggleFocusedSelection())
	case keys.BindingSlotMoveUp:
		return ran(m.moveFocusedSlot(-1))
	case keys.BindingSlotMoveDown:
		return ran(m.moveFocusedSlot(+1))
	case keys.BindingSlotDelete:
		return ran(m.removeFocusedSlot())
	case keys.BindingRenderWheel:
		return ran(m.dispatchRenderWheel())
	case keys.BindingJumpNow:
		return ran(m.jumpFloatingToNow())
	case keys.BindingToggleBack:
		return ran(m.toggleFloatingBack())
	case keys.BindingLogConsole:
		return ran(m.focusLog())
	case keys.BindingConsole:
		return ran(m.openConsole())
	case keys.BindingCopy:
		return ran(m.copyAll())
	case keys.BindingSaveCity:
		return ran(m.saveDefaultCity())
	case keys.BindingHouseDisplay:
		return ran(openHouseDisplay(m))
	case keys.BindingSessions:
		return ran(openSessionsList(m))
	case keys.BindingNewSession:
		return ran(resetSession(m))
	}
	return m, nil, false
}

// blocked is the reason the command cannot run now, "" when it can. It reads
// the model and changes nothing. Where a command's key is gated by its table
// entry's When, the question is the one the entry's facts are resolved from
// (mainSelects, canMoveSlotUp, canRemoveSlot, canAttachDetails), so the key,
// the bar and the reason never disagree.
func (m Model) blocked(id keys.BindingID) string {
	switch id {
	case keys.BindingAttach:
		if !m.canAttachDetails() {
			return whyNoDetails
		}
	case keys.BindingSelect:
		if !m.mainSelects() {
			return whyNothingToPick
		}
	case keys.BindingSlotMoveUp:
		switch {
		case !m.canRemoveSlot():
			return whyNotOnOpened
		case !m.canMoveSlotUp():
			return whyNoneAbove
		}
	case keys.BindingSlotMoveDown:
		switch {
		case !m.canRemoveSlot():
			return whyNotOnOpened
		case !m.canMoveSlotDown():
			return whyNoneBelow
		}
	case keys.BindingSlotDelete:
		if !m.canRemoveSlot() {
			return whyNotOnOpened
		}
	case keys.BindingJumpNow:
		if m.slots == nil {
			return whyNoCity
		}
	case keys.BindingToggleBack:
		switch {
		case m.slots == nil:
			return whyNoCity
		case m.previousFloatingTime.IsZero():
			return whyNoBack
		}
	case keys.BindingOpenCharts:
		// The picker sets the natal anchor of an overlay on a resolved
		// workspace; before the location resolves there is nothing to anchor to.
		if m.slots == nil || !m.place.HasTimezone() {
			return whyNoCity
		}
	case keys.BindingSaveChart:
		if why := m.withoutChart(); why != "" {
			return why
		}
		// Saving acts on the floating slot only.
		if !m.focusedSlot().Floating {
			return whyOnlyNow
		}
	case keys.BindingRenderWheel:
		return m.withoutChart()
	case keys.BindingHouses, keys.BindingZodiac:
		// `startup` names the phase and which modal is open, so a modal opened
		// before the first frame commits is overwritten by that commit.
		if m.startup != startupReady {
			return whyLoading
		}
		return m.withoutChart()
	case keys.BindingAspects:
		if m.startup != startupReady {
			return whyLoading
		}
		// A read that failed is a refusal with a cause (C10): the chooser will
		// not offer a set Sextile invented. A read that has not landed yet is
		// not a refusal; the opener takes the request and opens on arrival.
		if m.aspectRegistry == nil && m.aspectRegErr != nil {
			return whyAspectsUnread
		}
	case keys.BindingBodies:
		if m.startup != startupReady {
			return whyLoading
		}
		if m.objectRegistry == nil {
			if m.objectRegErr != nil {
				return whyBodiesUnread
			}
			// Not landed yet: the opener takes the request, as for aspects.
			return ""
		}
		// The ticks are what the chart draws; without that answer there is no
		// list to offer, and a guessed one would be worse.
		if _, err := m.lens.VisibleObjectNumbers(); err != nil {
			return whyDrawnUnread
		}
	case keys.BindingHouseDisplay:
		if m.startup != startupReady {
			return whyLoading
		}
	case keys.BindingSessions:
		if m.startup != startupReady {
			return whyLoading
		}
		if m.slots == nil {
			return whyNoCity
		}
	case keys.BindingNewSession:
		if m.startup != startupReady {
			return whyLoading
		}
		if m.slots == nil {
			return whyNoCity
		}
		if !m.named.running() && m.sessionIsEmpty() {
			return whySessionEmpty
		}
	case keys.BindingCopy:
		return m.copyBlocked()
	case keys.BindingSaveCity:
		// A second window writes nothing: the session that has to keep the open
		// charts' frames is the first window's.
		if !m.ownsSession() {
			return whyNotFirstWindow
		}
		if !m.place.HasTimezone() {
			return whyNoCity
		}
	}
	return ""
}

// withoutChart is why there is no chart under the cursor, "" when there is one.
func (m Model) withoutChart() string {
	switch {
	case m.slots == nil:
		return whyNoCity
	case m.focusedSlot() == nil:
		return whyNoChart
	}
	return ""
}
