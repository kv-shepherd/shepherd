package main

import "testing"

func TestMainDelegatesToServerBootstrap(t *testing.T) {
	t.Parallel()

	called := false
	original := runMain
	runMain = func() {
		called = true
	}
	defer func() {
		runMain = original
	}()

	main()

	if !called {
		t.Fatal("expected main to delegate to serverbootstrap.Main")
	}
}
