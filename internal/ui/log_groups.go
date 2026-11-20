package ui

import "strings"

// log_groups.go holds the groups of the Log panel: one closed set, each named
// as the screen names things. A scope is the word a call site passes to a Log
// writer and the large Log prints before a line; scopeGroups is the one table
// that maps every scope to its group.

// logGroup is the group column of a Log panel line.
type logGroup uint8

const (
	// groupNone is the group of a line that is not in the Log panel: an argv
	// line, the now line, a warning, a trace.
	groupNone logGroup = iota
	groupChart
	groupSession
	groupFolder
	groupSettings
	groupCity
	groupBodies
	groupAspects
	groupHouses
	groupZodiac
	groupDetails
	groupMoon
	groupTimeline
	groupWheel
	groupAstrolog
	groupSextile
)

// logGroupCells is the width of the group column; the message starts one cell
// after it.
const logGroupCells = 8

var logGroupNames = [...]string{
	groupNone: "", groupChart: "Chart", groupSession: "Session", groupFolder: "Folder",
	groupSettings: "Settings", groupCity: "City", groupBodies: "Bodies", groupAspects: "Aspects",
	groupHouses: "Houses", groupZodiac: "Zodiac", groupDetails: "Details", groupMoon: "Moon",
	groupTimeline: "Timeline", groupWheel: "Wheel", groupAstrolog: "Astrolog", groupSextile: "Sextile",
}

func (g logGroup) String() string { return logGroupNames[g] }

// cell is the group padded to the width of the column.
func (g logGroup) cell() string {
	s := g.String()
	return s + strings.Repeat(" ", max(logGroupCells-len(s), 0))
}

// scopeGroups maps each scope a Log writer is given to its group.
var scopeGroups = map[string]logGroup{
	"chart": groupChart, "save": groupChart, "slots": groupChart, "time": groupChart,
	"session": groupSession, "second window": groupSession, "journal": groupSession,
	"charts folder": groupFolder, "charts folder position": groupFolder,
	"settings": groupSettings, "defaults": groupSettings, "default city saved": groupSettings,
	"city":     groupCity,
	"bodies":   groupBodies,
	"aspects":  groupAspects,
	"houses":   groupHouses,
	"zodiac":   groupZodiac,
	"details":  groupDetails,
	"moon":     groupMoon,
	"timeline": groupTimeline,
	"wheel":    groupWheel,
	"astrolog": groupAstrolog,
	"launcher": groupSextile, "lock": groupSextile, "state": groupSextile, "template": groupSextile, "log": groupSextile,
}

// groupOf is the group of a scope. A scope the table lacks is a failure of
// TestEveryLogScopeHasAGroup; here it reads as Sextile's own.
func groupOf(scope string) logGroup {
	if g, ok := scopeGroups[scope]; ok {
		return g
	}
	return groupSextile
}
