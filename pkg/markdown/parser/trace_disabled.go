//go:build !diagnose

package parser

// traceRecorder is compiled into ordinary builds as a no-op. Keeping the call
// sites in the parser makes trace-enabled and trace-disabled parsing follow the
// exact same control flow.
type traceRecorder struct{}

func (t *traceRecorder) begin(*Parser)                           {}
func (t *traceRecorder) branch(string)                           {}
func (t *traceRecorder) finish(*Parser, []Event, string)         {}
func (t *traceRecorder) finalize(*Parser, finalizeMode, []Event) {}
