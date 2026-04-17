package sync

import "fmt"

type progressReporter interface {
	Printf(format string, args ...any)
	Warnf(format string, args ...any)
}

type stdoutReporter struct{}

func (stdoutReporter) Printf(format string, args ...any) {
	fmt.Printf(format, args...)
}

func (stdoutReporter) Warnf(format string, args ...any) {
	fmt.Printf("warning: "+format, args...)
}
