package kernel

// EvidenceLevel mirrors the certainty model defined by the SSOT.
type EvidenceLevel string

const (
	EvidenceConfirmed EvidenceLevel = "confirmed"
	EvidenceGuarded   EvidenceLevel = "guarded"
	EvidenceBlocked   EvidenceLevel = "blocked"
)
