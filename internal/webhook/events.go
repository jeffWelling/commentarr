// Package webhook dispatches event payloads to user-configured HTTP
// endpoints with retry + metrics.
package webhook

// Event is the set of triggers Commentarr supports (DESIGN.md § 5.11).
type Event string

const (
	EventSearch          Event = "OnSearch"
	EventGrab            Event = "OnGrab"
	EventDownload        Event = "OnDownload"
	EventImport          Event = "OnImport"
	EventReplace         Event = "OnReplace"
	EventTrash           Event = "OnTrash"
	EventTrashExpire     Event = "OnTrashExpire"
	EventRestore         Event = "OnRestore"
	EventVerifyFail      Event = "OnVerifyFail"
	EventSafetyViolation Event = "OnSafetyViolation"
	EventHealthIssue     Event = "OnHealthIssue"
	EventTest            Event = "OnTest"

	// EventUpgradeAvailable fires when a periodic re-search of a
	// resolved title turns up a candidate that scores higher than the
	// release Commentarr already imported. The operator decides whether
	// to grab the new release; Commentarr never auto-replaces a
	// successfully-imported file.
	//
	// Payload (see CONFIGURATION.md):
	//   title_id, current_release, current_score, candidate_release,
	//   candidate_score, candidate_indexer.
	EventUpgradeAvailable Event = "OnUpgradeAvailable"
)

// Envelope is the standard wrapper around every event payload.
type Envelope struct {
	EventType Event                  `json:"event_type"`
	Timestamp string                 `json:"timestamp"`
	Version   string                 `json:"version"`
	Payload   map[string]interface{} `json:"payload"`
}
