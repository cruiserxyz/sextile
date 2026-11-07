package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// The owner of the keyboard is a keys.Owner; ui spells it the short way.
type owner = keys.Owner

const (
	ownerMain        = keys.OwnerMain
	ownerCity        = keys.OwnerCity
	ownerOpenChart   = keys.OwnerOpenChart
	ownerChartTitle  = keys.OwnerChartTitle
	ownerSaveChart   = keys.OwnerSaveChart
	ownerBodies      = keys.OwnerBodies
	ownerHouses      = keys.OwnerHouses
	ownerZodiac      = keys.OwnerZodiac
	ownerAspects     = keys.OwnerAspects
	ownerHelp        = keys.OwnerHelp
	ownerConsole     = keys.OwnerConsole
	ownerDetails     = keys.OwnerDetails
	ownerDisplay     = keys.OwnerDisplay
	ownerSessions    = keys.OwnerSessions
	ownerSessionName = keys.OwnerSessionName
	ownerActions     = keys.OwnerActions
	ownerLog         = keys.OwnerLog
	ownerExperiments = keys.OwnerExperiments
)

// keysContext is the context the keys package draws this frame's bar from:
// the current owner's, with the footer's right-hand slot as it stands.
func (m Model) keysContext() keys.Context {
	c := m.keysContextFor(m.currentOwner())
	c.Footer = m.footerIndicator()
	c.FooterNotice = isNotice(c.Footer)
	return c
}

// keysContextFor resolves the context for owner o: o's table, and the answers
// to the conditions o's entries ask, each read once from the program. Only o's
// group is filled, so a condition of another owner costs nothing and reads
// false. The leader and the style are always there.
func (m Model) keysContextFor(o owner) keys.Context {
	c := m.keysFacts(o)
	c.Table = m.ownerBindings(o)
	return c
}

// keysFacts is keysContextFor without the table: what the conditions of o's
// entries read, for a caller that already holds the entries.
func (m Model) keysFacts(o owner) keys.Context {
	c := keys.Context{
		Owner:   o,
		Variant: m.variant(o),
		Style:   m.cfg,
		Leader: keys.LeaderFacts{
			Pending: m.leader.pending, Seq: m.leader.seq, Key: m.leaderKey,
			Bindings: m.leaderBindings, Tree: m.leaderMap, Timeout: m.leaderTimeout,
		},
	}
	switch o {
	case ownerMain:
		c.Main = keys.MainFacts{
			Selects: m.mainSelects(), HasWorkspace: m.hasWorkspace(), Moves: m.mainMoves(), Pages: m.mainPages(),
			CanMoveUp: m.canMoveSlotUp(), CanMoveDown: m.canMoveSlotDown(), CanRemove: m.canRemoveSlot(),
			DetailsScrolls: m.detailsScrolls(), HoldsAChart: m.holdsAChart(),
		}
		c.Attach = keys.AttachFacts{CanAttach: m.canAttachDetails(), Title: m.attachTitle()}
	case ownerHelp:
		// Help lists the main table and every popup's: it needs the tables, the
		// one main condition it asks (HelpWhen) and Enter's title.
		c.Main.HoldsAChart = m.holdsAChart()
		c.Attach = keys.AttachFacts{Title: m.attachTitle()}
		c.Help = keys.HelpFacts{
			Main:       mainBindings(m.keys),
			LeaderOnly: leaderOnlyBindings(),
			Popups:     map[keys.Owner][]keys.BindingEntry{},
			State:      m.help,
		}
		for _, po := range keys.HelpPopupOwners() {
			c.Help.Popups[po] = m.ownerBindings(po)
		}
	case ownerDetails:
		c.Details = keys.DetailsFacts{
			Moves: m.detailsMoves(), Scrolls: m.detailsScrolls(), OnTimeline: m.detailsOnTimeline(),
			OnTimelineRows: m.detailsOnTimelineRows(), Filterable: m.detailsFilterable(), HasSeveralTabs: m.detailsHasSeveralTabs(),
		}
	case ownerCity, ownerBodies, ownerAspects, ownerHouses, ownerZodiac, ownerActions:
		c.Popup.Rows = m.popupRows()
	case ownerOpenChart:
		c.Popup.Rows = m.popupRows()
		c.Picker.Renames = m.pickerRenames()
	case ownerSessions:
		c.Popup.Rows = m.popupRows()
		c.Sessions = keys.SessionsFacts{
			OnSaveRow: m.sessionsOnSaveRow(), CanOpen: m.sessionsCanOpen(),
			OnFreeRow: m.sessionsOnFreeRow(), CanTrash: m.sessionsCanTrash(),
		}
	case ownerSessionName:
		c.Sessions.NameTyped = m.sessionNameTyped()
	case ownerSaveChart:
		c.Save.HasName = m.saveHasName()
	case ownerConsole:
		c.Console.Lines = len(m.console.snapshot)
	case ownerLog:
		c.Log.Lines = len(m.logPanelLines())
	case ownerDisplay:
		c.Display.CanChoose = m.displayCanChoose()
	}
	return c
}

// variant is which of owner o's tables is in force; ownerBindings reads it.
func (m Model) variant(o owner) keys.Variant {
	switch o {
	case ownerChartTitle:
		if m.title.Confirming {
			return keys.VariantTitleConfirm
		}
	case ownerDetails:
		if m.timeline.searching && m.timelineSearchLine() {
			return keys.VariantTimelineSearch
		}
		if m.detailsReportSearchOpen() {
			return keys.VariantReportSearch
		}
	}
	return keys.VariantDefault
}

// available drops the entries of owner o's table that do not hold now.
func (m Model) available(o owner, entries []keys.BindingEntry) []keys.BindingEntry {
	return keys.Available(m.keysFacts(o), entries)
}

// keyBarEntries is what the bar shows this frame: the current owner's table,
// as far as it is available now, each entry with its bar word.
func (m Model) keyBarEntries() []keys.BindingEntry {
	return keys.BarEntries(m.keysContext())
}
