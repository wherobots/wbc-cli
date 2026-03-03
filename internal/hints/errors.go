package hints

import (
	"fmt"

	"wherobots/cli/internal/spec"
)

type UsageError struct {
	Operation *spec.Operation
	Cause     error
}

func (e *UsageError) Error() string {
	return Format(e.Operation, e.Cause)
}

func (e *UsageError) Unwrap() error {
	return e.Cause
}

func Wrap(op *spec.Operation, cause error) error {
	if cause == nil {
		return nil
	}
	return &UsageError{Operation: op, Cause: cause}
}

func Format(op *spec.Operation, cause error) string {
	if op == nil {
		return fmt.Sprintf("Command failed. %v", cause)
	}

	return fmt.Sprintf(
		"Command failed. %v Did you mean to use the body: %s? Required Path Params: %s Required Body Params: %s Expected Types: %s",
		cause,
		BuildBodyTemplate(op),
		RequiredPathParams(op),
		RequiredBodyParams(op),
		ExpectedTypeSummary(op),
	)
}
