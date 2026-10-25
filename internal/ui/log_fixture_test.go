package ui

import "fmt"

// testNote and testError put a line in the Log exactly as given, for tests
// whose subject is the Log's lanes, windows and rendering rather than the
// form of a line. Production writes only through logNote and logError
// (the specification A3, A4); a test file is outside that scan.
func (m Model) testNote(format string, args ...any) Model {
	return m.appendLog(laneNote, fmt.Sprintf(format, args...))
}

func (m Model) testError(format string, args ...any) Model {
	return m.appendLog(laneError, fmt.Sprintf(format, args...))
}
