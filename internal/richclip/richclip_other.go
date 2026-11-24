//go:build !darwin && !linux && !windows

package richclip

// Other platforms have no rich clipboard, so copy degrades to plain text.
type system struct{}

func (system) Set(string, string) error          { return ErrUnsupported }
func (system) ReadHTML() (string, string, error) { return "", "", ErrUnsupported }
