// Command djangogo is the Djan-Go-Go command line, the counterpart of
// django-admin: "djangogo startproject mysite" creates a project, and the
// main package of the project then runs its management commands.
package main

import (
	"fmt"
	"os"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/conf"
)

func main() {
	os.Exit(run())
}

// run builds an application without apps or database and runs the command
// the arguments name. It returns the exit code instead of calling os.Exit,
// so that the deferred Close runs.
func run() int {
	app, err := djangogo.New(conf.Settings{
		Debug:     true,
		SecretKey: "djangogo-command-line",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = app.Close() }()
	return app.Run(os.Args[1:])
}
