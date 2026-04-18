package ui

// Single-glyph markers used in CLI reports. The glyph carries meaning; colour
// comes from the Style paired with it (markers.go + styles.go are a tuple).
//
//	Marker       Glyph  Style         Meaning
//	MarkOK       ✓      StyleSuccess  satisfied / succeeded / present
//	MarkFail     ✗      StyleError    hard failure / check failed
//	MarkAbsent   ✗      StyleHint     missing / disabled (neutral, no failure)
//	MarkPending  →      StyleWarning  pending change / not yet applied
//	MarkWarn     ⚠      StyleWarning  attention needed, non-fatal
//	MarkStarred  ★      StyleSuccess  active / suggested choice
//
// Callers should not inline glyph literals; use these constants so the marker
// alphabet stays consistent and searchable across commands.
const (
	MarkOK      = "✓"
	MarkFail    = "✗"
	MarkAbsent  = "✗"
	MarkPending = "→"
	MarkWarn    = "⚠"
	MarkStarred = "★"
)

// OKMark returns the OK glyph styled for success.
func OKMark() string { return StyleSuccess.Render(MarkOK) }

// FailMark returns the fail glyph styled as a hard failure.
func FailMark() string { return StyleError.Render(MarkFail) }

// AbsentMark returns the absent glyph styled as neutral-missing.
func AbsentMark() string { return StyleHint.Render(MarkAbsent) }

// PendingMark returns the pending-change arrow styled as warning.
func PendingMark() string { return StyleWarning.Render(MarkPending) }

// WarnMark returns the warning glyph styled as warning.
func WarnMark() string { return StyleWarning.Render(MarkWarn) }
