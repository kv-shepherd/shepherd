package main

import (
	_ "example.com/authproviderplugin-external-smoke/plugins/example"

	"kv-shepherd.io/shepherd/pkg/serverbootstrap"
)

func main() {
	serverbootstrap.Main()
}
