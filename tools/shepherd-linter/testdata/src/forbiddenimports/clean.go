// Package forbiddenimports is the test data for the forbiddenimports analyzer.
// This package intentionally has no forbidden imports to test the clean path.
package forbiddenimports

import "fmt"

// Clean is a function with no violations.
func Clean() {
	fmt.Println("clean package")
}
