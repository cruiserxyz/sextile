package events

// Event is a control-plane signal. The event-loop orchestrator (deferred per
// sxt-tick) reads Events and advances the IR walk one step per signal; other
// packages may emit Events and, later, listen for them. An Event is
// payload-free by construction: the enum type carries no associated data, so
// a value cannot smuggle astrological content across the control plane.
//
// The enum is extensible — new constants may be added as new coordination
// points surface. Consumers should treat an unknown Event value as a no-op
// rather than panicking, so additions do not break existing listeners.
type Event uint8

const (
	EventUnknown Event = iota
	// IRReady — ir has produced an IRPacket. The orchestrator should
	// pulse astrolog on the next beat.
	IRReady
	// OutputReady — astrolog has returned a RawOutput. The orchestrator should
	// advance payload upward so lookup can retrieve.
	OutputReady
	// FramePainted — ui has finished a render. The walk is complete.
	FramePainted
	// Cancel — a newer intent has arrived. The orchestrator finishes the current
	// beat atomically, then abandons the rest of the walk (last-wins).
	Cancel
	// ErrorReady — a package has signalled an error. The orchestrator honours it
	// between beats, same treatment as Cancel: finish the current beat,
	// abandon the walk, surface the error at the perimeter.
	ErrorReady
)

// String returns a stable snake_case name for the Event. Diagnostics only;
// the wire representation (when an event bus eventually serialises Events)
// speaks the enum's numeric value.
func (e Event) String() string {
	switch e {
	case IRReady:
		return "ir_ready"
	case OutputReady:
		return "output_ready"
	case FramePainted:
		return "frame_painted"
	case Cancel:
		return "cancel"
	case ErrorReady:
		return "error_ready"
	default:
		return "unknown"
	}
}
