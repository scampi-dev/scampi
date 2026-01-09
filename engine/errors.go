package engine

type AbortError struct {
	Causes []error
}

func (AbortError) Error() string {
	return "execution aborted"
}
