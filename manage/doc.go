// Package manage provides the management-command dispatcher, the counterpart
// of django.core.management: the ManagementUtility (dispatch by name, the main
// help, the "Unknown command" error) and the parts of BaseCommand that are
// not specific to one command (streams, argument parsing, CommandError, exit
// codes, the system-check hook).
//
// # Mapping from Django
//
//	class Command(BaseCommand)             a type that implements Command
//	help = "..."                           Command.Help
//	add_arguments(self, parser)            Flagger.Flags(fs *flag.FlagSet)
//	parser.add_argument("args", nargs="*") Usager.Usage, the synopsis of the positional arguments
//	handle(self, *args, **options)         Command.Run(args); the options are the variables the flags are bound to
//	self.stdout, self.stderr               Base.Stdout, Base.Stderr
//	requires_system_checks = []            CheckSkipper.SkipSystemChecks
//	raise CommandError("...")              return manage.Errorf("...")
//	CommandError("...", returncode=3)      &manage.CommandError{Msg: "...", Code: 3}
//	parser.error("...")                    return &manage.UsageError{Msg: "..."}
//	INSTALLED_APPS command discovery       Registry.RegisterApp, called in reverse INSTALLED_APPS order
//	execute_from_command_line(sys.argv)    os.Exit(registry.Run(os.Args[1:]))
//	ManagementUtility.main_help_text       Registry.MainHelp
//	BaseCommand.print_help                 Registry.CommandHelp
//
// # Differences from Django
//
// The standard flag package parses the options, not argparse. Options and
// positional arguments may be mixed and "--" ends the options, as with
// argparse, and the messages of the usage errors are argparse's. The rest is
// the flag package: an option is found by its full name with one or two
// dashes, there are no abbreviations and no combined short options, and an
// option that takes a value takes the next argument whatever it looks like.
// Two flags bound to the same variable are one option with an alias
// ("-t, --tag TAG" in the help).
//
// The usage of a command is one line, "usage: prog name [options] synopsis",
// and the help lists the options sorted by name, because a flag set does not
// keep the order of declaration.
//
// A command that takes flags and does not implement Usager takes no
// positional arguments: one given on the command line is a usage error
// ("unrecognized arguments"), as for a Django command without positional
// arguments.
//
// The only option common to all commands is --skip-checks; it exists for
// every command that requires the system checks, and the help lists it while
// Registry.SystemChecks is set. There is no call_command: Registry.Execute is
// the one entry point, and it runs the system checks unless --skip-checks is
// given.
package manage
