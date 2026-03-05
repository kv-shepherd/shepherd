// Package riverbypass is the test data for the riverbypass analyzer.
// This package intentionally has no violations to test the clean path.
package riverbypass

import "fmt"

// Clean is a function with no violations.
func Clean() {
	fmt.Println("clean package, no direct DB writes")
}
