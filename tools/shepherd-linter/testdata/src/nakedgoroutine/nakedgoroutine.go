// Package nakedgoroutine is the test data for the nakedgoroutine analyzer.
package nakedgoroutine

import "fmt"

// BannedFunc contains a naked goroutine that should be flagged.
func BannedFunc() {
	go fmt.Println("naked goroutine") // want `naked goroutine is forbidden`
}

// AllowedFunc simulates a worker pool submission (no `go` statement).
func AllowedFunc(submit func(func())) {
	submit(func() {
		fmt.Println("via worker pool")
	})
}
