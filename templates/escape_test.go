package templates

import (
	"testing"
	"time"

	"github.com/flosch/pongo2/v6"
)

// TestIsSafe guards the reflection behind isSafe. Should pongo2 rename the
// unexported field that holds the safe mark, every value would count as not
// safe and be escaped twice; this test reports that before a template does.
func TestIsSafe(t *testing.T) {
	if !isSafe(pongo2.AsSafeValue("<b>")) {
		t.Error("isSafe(AsSafeValue) = false; pongo2 renamed the safe field")
	}
	if isSafe(pongo2.AsValue("<b>")) {
		t.Error("isSafe(AsValue) = true, want false")
	}
	if isSafe(nil) {
		t.Error("isSafe(nil) = true, want false")
	}
}

// TestNowTagUsesClock renders {% now %} at a fixed time, which the ported
// Django tests cannot do because they compare with the real clock.
func TestNowTagUsesClock(t *testing.T) {
	saved := timeNow
	t.Cleanup(func() { timeNow = saved })
	timeNow = func() time.Time { return time.Date(2008, 1, 2, 3, 4, 5, 0, time.UTC) }

	cases := []struct{ src, want string }{
		{`{% now "Y-m-d H:i:s" %}`, "2008-01-02 03:04:05"},
		{`{% now "jS F Y" %}`, "2nd January 2008"},
		{`{% now "Y" as year %}[{{ year }}]`, "[2008]"},
	}
	for _, c := range cases {
		got, err := New().RenderString(c.src, nil)
		if err != nil {
			t.Fatalf("RenderString(%q): %v", c.src, err)
		}
		if got != c.want {
			t.Errorf("RenderString(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}
