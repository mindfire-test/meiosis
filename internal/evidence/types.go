package evidence

import "time"

type testEvent struct {
	Time    time.Time `json:"Time,omitempty"`
	Action  string    `json:"Action"`
	Package string    `json:"Package,omitempty"`
	Test    string    `json:"Test,omitempty"`
	Output  string    `json:"Output,omitempty"`
	Elapsed float64   `json:"Elapsed,omitempty"`
}

type testPayload struct {
	Format string      `json:"format"`
	Events []testEvent `json:"events"`
}
