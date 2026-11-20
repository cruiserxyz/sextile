package ui

// The Details lists. The Timeline and the Moon list are two instances of one
// list: every behaviour is written once, for "a list", and a timelineList says
// which instance a call is about. This file is the one place that declares the
// lists and names the Moon one; everything else asks a list for its tab, its
// surface, its title and its nouns.

// timelineList names one instance of the Details list.
type timelineList uint8

const (
	listTimeline timelineList = iota
	listMoon
)

// allLists is every list, in tab order.
var allLists = []timelineList{listTimeline, listMoon}

// tab is the Details tab that shows the list.
func (k timelineList) tab() TabID {
	if k == listMoon {
		return TabDetailsMoon
	}
	return TabDetailsTimeline
}

// title is the tab's title.
func (k timelineList) title() string {
	if k == listMoon {
		return tabTitleMoon
	}
	return tabTitleTimeline
}

// surface is the refresh surface that searches the list.
func (k timelineList) surface() surface {
	if k == listMoon {
		return surfaceMoon
	}
	return surfaceTimeline
}

// noun is how the Log names the list: "the timeline not read".
func (k timelineList) noun() string {
	if k == listMoon {
		return "the Moon list"
	}
	return "the timeline"
}

// scope is the Log scope of the list's `,y` note.
func (k timelineList) scope() string {
	if k == listMoon {
		return "moon"
	}
	return "timeline"
}

// moon says whether the search is limited to the Moon.
func (k timelineList) moon() bool { return k == listMoon }

// listOfTab is the list a Details tab shows, if it shows one.
func listOfTab(id TabID) (timelineList, bool) {
	for _, k := range allLists {
		if k.tab() == id {
			return k, true
		}
	}
	return listTimeline, false
}

// isListTab says a Details tab shows a list.
func isListTab(id TabID) bool {
	_, ok := listOfTab(id)
	return ok
}

// list is the state of list k.
func (m *Model) list(k timelineList) *timelineState {
	if k == listMoon {
		return &m.moonList
	}
	return &m.timeline
}

// shownList is the list of the active Details tab; the Timeline when the
// active tab is not a list.
func (m Model) shownList() timelineList {
	k, _ := listOfTab(m.activeTabID(PanelDetails))
	return k
}
