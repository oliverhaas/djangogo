package checks_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/djangotest"
)

// The messages of the report tests. The expected texts are what the check
// command of Django 6.0.3 prints for a check that returns the same messages.
var (
	reportWarning  = checks.Warning("A warning", checks.Hint("A hint"), checks.ID("app.W001"))
	reportWarning2 = checks.Warning("Second warning", checks.Obj(dummyObj{}), checks.ID("app.W002"))
	reportError    = checks.Error("An error", checks.Hint("Fix it"), checks.Obj(dummyObj{}), checks.ID("app.E001"))
	reportError2   = checks.Error("Another error", checks.ID("app.E002"))
	reportCritical = checks.Critical("A critical", checks.ID("app.C001"))
	reportInfo     = checks.Info("An info", checks.ID("app.I001"))
	reportDebug    = checks.Debug("A debug")
)

// TestSummarize compares the report with the output of Django's check command:
// the header, the groups in the order of their severity with sorted lines, the
// blank lines between them and the three forms of the last line.
func TestSummarize(t *testing.T) {
	const warningGroup = "System check identified some issues:\n" +
		"\nWARNINGS:\n?: (app.W001) A warning\n\tHINT: A hint\n"
	const errorGroup = "System check identified some issues:\n" +
		"\nERRORS:\nobj: (app.E001) An error\n\tHINT: Fix it\n"

	tests := []struct {
		name             string
		messages         []checks.Message
		silenced         []string
		failLevel        checks.Level
		displayNumErrors bool
		want             checks.Report
	}{
		{
			name:             "no messages",
			failLevel:        checks.LevelError,
			displayNumErrors: true,
			want:             checks.Report{Text: "System check identified no issues (0 silenced).\n"},
		},
		{
			name:      "no messages and no count",
			failLevel: checks.LevelError,
			want:      checks.Report{},
		},
		{
			name:      "a warning without the count",
			messages:  []checks.Message{reportWarning},
			failLevel: checks.LevelError,
			want:      checks.Report{Text: warningGroup, Issues: 1},
		},
		{
			name:             "a warning",
			messages:         []checks.Message{reportWarning},
			failLevel:        checks.LevelError,
			displayNumErrors: true,
			want: checks.Report{
				Text:   warningGroup + "\nSystem check identified 1 issue (0 silenced).\n",
				Issues: 1,
			},
		},
		{
			name:             "a warning at the fail level WARNING",
			messages:         []checks.Message{reportWarning},
			failLevel:        checks.LevelWarning,
			displayNumErrors: true,
			want: checks.Report{
				Text:   warningGroup + "\nSystem check identified 1 issue (0 silenced).\n",
				Failed: true,
				Issues: 1,
			},
		},
		{
			name:             "a silenced warning at the fail level WARNING",
			messages:         []checks.Message{reportWarning},
			silenced:         []string{"app.W001"},
			failLevel:        checks.LevelWarning,
			displayNumErrors: true,
			want:             checks.Report{Text: "System check identified no issues (1 silenced).\n"},
		},
		{
			name:             "an error",
			messages:         []checks.Message{reportError},
			failLevel:        checks.LevelError,
			displayNumErrors: true,
			want: checks.Report{
				Text:   errorGroup + "\nSystem check identified 1 issue (0 silenced).\n",
				Failed: true,
				Issues: 1,
			},
		},
		{
			name:      "an error without the count",
			messages:  []checks.Message{reportError},
			failLevel: checks.LevelError,
			want:      checks.Report{Text: errorGroup, Failed: true, Issues: 1},
		},
		{
			name:             "a warning and a silenced error",
			messages:         []checks.Message{reportWarning, reportError},
			silenced:         []string{"app.E001"},
			failLevel:        checks.LevelError,
			displayNumErrors: true,
			want: checks.Report{
				Text:   warningGroup + "\nSystem check identified 1 issue (1 silenced).\n",
				Issues: 1,
			},
		},
		{
			name: "every level",
			messages: []checks.Message{
				reportWarning, reportWarning2, reportError, reportError2, reportCritical, reportInfo, reportDebug,
			},
			failLevel:        checks.LevelError,
			displayNumErrors: true,
			want: checks.Report{
				Text: "System check identified some issues:\n" +
					"\nCRITICALS:\n?: (app.C001) A critical\n" +
					"\nERRORS:\n?: (app.E002) Another error\nobj: (app.E001) An error\n\tHINT: Fix it\n" +
					"\nWARNINGS:\n?: (app.W001) A warning\n\tHINT: A hint\nobj: (app.W002) Second warning\n" +
					"\nINFOS:\n?: (app.I001) An info\n" +
					"\nDEBUGS:\n?: A debug\n" +
					"\nSystem check identified 7 issues (0 silenced).\n",
				Failed: true,
				Issues: 7,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checks.Summarize(tt.messages, tt.silenced, tt.failLevel, tt.displayNumErrors)
			djangotest.AssertEqual(t, got, tt.want)
		})
	}
}

// TestSummarizeKeepsMessageWithoutID shows a message without an id whatever
// the silenced list holds: there is no id to silence it by.
func TestSummarizeKeepsMessageWithoutID(t *testing.T) {
	report := checks.Summarize([]checks.Message{reportDebug}, []string{""}, checks.LevelError, false)

	djangotest.AssertEqual(t, report.Issues, 1)
}

// TestSystemCheckErrorText prints the report of a failed run as Django's check
// command does, once the caller has put the name of the error in front.
func TestSystemCheckErrorText(t *testing.T) {
	report := checks.Summarize([]checks.Message{reportError}, nil, checks.LevelError, true)
	var err error = &checks.SystemCheckError{Report: report}

	djangotest.AssertEqual(t, "SystemCheckError: "+err.Error()+"\n",
		"SystemCheckError: System check identified some issues:\n"+
			"\nERRORS:\nobj: (app.E001) An error\n\tHINT: Fix it\n"+
			"\nSystem check identified 1 issue (0 silenced).\n")
}
