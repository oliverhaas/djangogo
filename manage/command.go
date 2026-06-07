// Package manage provides the management-command dispatcher (Django's manage.py).
package manage

// Command is a management subcommand.
type Command interface {
	Name() string
	Help() string
	Run(args []string) error
}
