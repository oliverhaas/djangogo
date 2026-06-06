// Command djangogo is the Djan-Go-Go management CLI: the manage-style
// entrypoint for startproject, startapp, runserver, makemigrations, migrate,
// createsuperuser, and per-app commands.
//
// This is a placeholder; commands land as the framework is built (see PLAN.md).
package main

import (
	"fmt"
	"os"

	"github.com/oliverhaas/djangogo"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("djangogo %s\n\nusage: djangogo <command>\n", djangogo.Version)
		fmt.Println("(commands land as the framework is built; see PLAN.md)")
		return
	}

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Printf("djangogo %s\n", djangogo.Version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (not implemented yet; see PLAN.md)\n", os.Args[1])
		os.Exit(2)
	}
}
