package watch

// ObservationError means generation could not verify its inputs before
// publication. Retry only after observation is valid again, even if its content
// snapshot equals the one used for the unpublished generation. Ordinary parser
// and renderer failures do not use this marker and never retry unchanged input.
type ObservationError struct{ Err error }

func (e *ObservationError) Error() string { return e.Err.Error() }
func (e *ObservationError) Unwrap() error { return e.Err }
