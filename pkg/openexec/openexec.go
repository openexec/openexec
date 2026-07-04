// Package openexec is the public entrypoint for building an OpenExec binary. The
// open-source binary (cmd/openexec) calls Main directly. A downstream build that
// bundles modules imports this package plus the module's registration package
// (which calls pkg/plugin.Register* from an init()), so the module's tools and
// commands are wired in without core importing the module:
//
//	package main
//	import (
//	    _ "example.com/openexec-governance/register" // init() registers plugins
//	    "github.com/openexec/openexec/pkg/openexec"
//	)
//	func main() { openexec.Main() }
package openexec

import "github.com/openexec/openexec/internal/cli"

// Main runs the OpenExec CLI. Module plugins registered via pkg/plugin before
// this call (i.e. in init()) are attached automatically.
func Main() { cli.Execute() }
