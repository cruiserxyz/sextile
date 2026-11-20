package ui

import "fmt"

// testNote and testError put a line in the Log as given, with no scope in its
// stored text and the Session group in the panel, for tests
// whose subject is the Log's lanes, windows and rendering rather than the
// form of a line. Production writes only through logNote and logError
// (the specification A3, A4); a test file is outside that scan.
func (m Model) testNote(format string, args ...any) Model {
	text := fmt.Sprintf(format, args...)
	return m.appendLogLine(logLine{lane: laneNote, text: text, group: groupSession, msg: text})
}

func (m Model) testError(format string, args ...any) Model {
	text := fmt.Sprintf(format, args...)
	return m.appendLogLine(logLine{lane: laneError, text: text, group: groupSession, msg: text})
}
