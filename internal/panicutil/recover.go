package panicutil

import (
	"fmt"
	"os"
	"runtime/debug"
)

// Recover logs a panic and stack trace to stderr without crashing the process.
// Usage: defer panicutil.Recover("label")
func Recover(context string) {
	if r := recover(); r != nil {
		fmt.Fprintf(os.Stderr, "humrun: panic in %s: %v\n%s\n", context, r, debug.Stack())
	}
}
