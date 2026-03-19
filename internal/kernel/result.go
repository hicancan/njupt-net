package kernel

// RawCapture keeps response evidence for diagnostics and machine output.
type RawCapture struct {
	Status   int                 `json:"status,omitempty"`
	Headers  map[string][]string `json:"headers,omitempty"`
	Body     string              `json:"body,omitempty"`
	FinalURL string              `json:"finalURL,omitempty"`
}

// OperationResult is the canonical typed result returned by kernel/app actions.
type OperationResult[T any] struct {
	Level    EvidenceLevel `json:"level"`
	Success  bool          `json:"success"`
	Message  string        `json:"message,omitempty"`
	Data     *T            `json:"data,omitempty"`
	Problems []Problem     `json:"problems,omitempty"`
	Raw      *RawCapture   `json:"raw,omitempty"`
}

// WriteBackResult captures pre/target/post/restore evidence for stateful writes.
type WriteBackResult struct {
	PreState      map[string]string `json:"preState,omitempty"`
	TargetState   map[string]string `json:"targetState,omitempty"`
	PostState     map[string]string `json:"postState,omitempty"`
	RestoredState map[string]string `json:"restoredState,omitempty"`
}
