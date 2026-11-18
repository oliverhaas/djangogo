package checks

import (
	"fmt"
	"slices"
	"strings"
)

// Report is the outcome of a run as Django's check command presents it
// (BaseCommand.check).
type Report struct {
	// Text is the output of the command as it reaches the stream: the header
	// "System check identified some issues:", the groups of messages
	// (CRITICALS, ERRORS, WARNINGS, INFOS, DEBUGS) and the line with the
	// number of issues, or only that line when there is no issue to show. It
	// ends with a line end, except that it is empty when there is nothing to
	// print. Django writes it to stderr when Issues is not zero and to stdout
	// otherwise.
	Text string
	// Failed reports that a message at or above the fail level is not
	// silenced: the command must stop with a SystemCheckError.
	Failed bool
	// Issues is the number of messages shown, which leaves out the silenced
	// ones.
	Issues int
}

// levelGroups lists the groups of a report in the order of their output, each
// with the lowest level that belongs to it.
var levelGroups = []struct {
	name string
	from Level
}{
	{"CRITICALS", LevelCritical},
	{"ERRORS", LevelError},
	{"WARNINGS", LevelWarning},
	{"INFOS", LevelInfo},
}

// debugGroup is the group of every level below LevelInfo.
const debugGroup = "DEBUGS"

// groupIndex returns the index of the group of level in a report: the index in
// levelGroups, or len(levelGroups) for the debug group.
func groupIndex(level Level) int {
	for i, g := range levelGroups {
		if level >= g.from {
			return i
		}
	}
	return len(levelGroups)
}

// Summarize formats the messages of a run as Django's BaseCommand.check does.
// It leaves out the messages whose id is in silenced (the value of
// SILENCED_SYSTEM_CHECKS), groups the others by level, sorts the lines of each
// group, and reports a failure when a message that is not silenced is at or
// above failLevel (check --fail-level, LevelError by default).
//
// displayNumErrors adds the last line, "System check identified 1 issue (0
// silenced)." in its three forms, as Django's display_num_errors does. The
// check command sets it; a command that only runs the checks before its own
// work does not, so it prints nothing when there is no issue.
func Summarize(messages []Message, silenced []string, failLevel Level, displayNumErrors bool) Report {
	var report Report
	groups := make([][]string, len(levelGroups)+1)
	for _, m := range messages {
		if m.IsSilenced(silenced) {
			continue
		}
		report.Issues++
		if m.IsSerious(failLevel) {
			report.Failed = true
		}
		i := groupIndex(m.Level)
		groups[i] = append(groups[i], m.String())
	}

	var b strings.Builder
	if report.Issues > 0 {
		b.WriteString("System check identified some issues:\n")
	}
	for i, lines := range groups {
		if len(lines) == 0 {
			continue
		}
		name := debugGroup
		if i < len(levelGroups) {
			name = levelGroups[i].name
		}
		slices.Sort(lines)
		b.WriteString("\n" + name + ":\n" + strings.Join(lines, "\n") + "\n")
	}
	if displayNumErrors {
		if report.Issues > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "System check identified %s (%d silenced).\n", issueCount(report.Issues), len(messages)-report.Issues)
	}
	report.Text = b.String()
	return report
}

// issueCount returns the number of issues in the words of Django's last report
// line: "no issues", "1 issue" or "<n> issues".
func issueCount(n int) string {
	switch n {
	case 0:
		return "no issues"
	case 1:
		return "1 issue"
	default:
		return fmt.Sprintf("%d issues", n)
	}
}

// SystemCheckError is the error of a failed run (Django's SystemCheckError).
type SystemCheckError struct {
	// Report is the report of the run.
	Report Report
}

// Error returns the text of the report without its final line end. Django
// prints "SystemCheckError: " before it; the caller adds that prefix.
func (e *SystemCheckError) Error() string {
	return strings.TrimSuffix(e.Report.Text, "\n")
}
