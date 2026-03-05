// Package service is a testdata package under internal/ that contains violations.
package service

import "fmt"

// BannedFunc contains a naked goroutine that should be flagged.
func BannedFunc() {
	go fmt.Println("naked goroutine") // want `naked goroutine is forbidden`
}

// AllowedFunc simulates a worker pool submission (no naked go statement).
func AllowedFunc(submit func(func())) {
	submit(func() {
		fmt.Println("via worker pool")
	})
}

// SuppressedFunc uses the plugin-level suppression tag.
func SuppressedFunc() { //nolint:shepherd-arch
	go fmt.Println("suppressed goroutine")
}

// SuppressedLegacyTag keeps compatibility with legacy script suppression spelling.
func SuppressedLegacyTag() { //nolint:naked-goroutine
	go fmt.Println("suppressed with legacy tag")
}
