// Package autoreg loads auth-provider plugins through side-effect imports.
//
// This package is imported once by the composition root so plugin packages can
// self-register adapters in init() using the public plugin contract package.
package autoreg

import (
	_ "kv-shepherd.io/shepherd/plugins/authprovider/example"
)
