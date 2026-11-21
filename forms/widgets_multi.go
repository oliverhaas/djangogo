package forms

import (
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MultiValueWidget is a widget made of subwidgets, such as MultiWidget. A
// MultiValueField splits its values with SubValues, and a BoundField renders
// it with RenderRequired when its subfields decide the required attribute.
// A widget outside this package implements it to lay out its subwidgets in
// its own HTML, as the template of a Django MultiWidget subclass does.
type MultiValueWidget interface {
	Widget
	// Subwidgets returns the subwidgets in order.
	Subwidgets() []Widget
	// SubValues returns the values of the subwidgets for value: value itself
	// when it is a list, otherwise its decompressed parts.
	SubValues(value any) []any
	// RenderRequired is Render with the required attribute of each subwidget
	// given in required, in order; a nil required leaves the attributes as
	// they are.
	RenderRequired(name string, value any, attrs Attrs, required []bool) string
}

// MultiWidget renders several widgets side by side as one, Django's
// MultiWidget. Its value is a list with one value per subwidget; any other
// value is split with Decompress. Subwidget i is named after the field with
// the suffix _<Names[i]>, or _<i> when Names is nil; an empty name adds no
// suffix. A MultiValueField reads the list of values it submits.
type MultiWidget struct {
	// Widgets are the subwidgets in order.
	Widgets []Widget
	// Names are the suffixes of the subwidget names, Django's widgets given
	// as a dictionary.
	Names []string
	// Attrs are the HTML attributes of every subwidget. An id gets the
	// suffix _<i> for subwidget i, and a type replaces the input type of
	// each.
	Attrs Attrs
	// Decompress splits a value that is not a list into the values of the
	// subwidgets, Django's decompress. Without it such a value shows empty
	// subwidgets.
	Decompress func(value any) []any
}

// suffix returns the suffix of the name of subwidget i.
func (w MultiWidget) suffix(i int) string {
	if w.Names == nil {
		return "_" + strconv.Itoa(i)
	}
	if i < len(w.Names) && w.Names[i] != "" {
		return "_" + w.Names[i]
	}
	return ""
}

// Subwidgets implements MultiValueWidget.
func (w MultiWidget) Subwidgets() []Widget { return w.Widgets }

// SubValues implements MultiValueWidget.
func (w MultiWidget) SubValues(value any) []any {
	if list, ok := asList(value); ok {
		return list
	}
	if w.Decompress != nil {
		return w.Decompress(value)
	}
	return nil
}

// Render renders the subwidgets one after the other, with no space between
// them.
func (w MultiWidget) Render(name string, value any, attrs Attrs) string {
	return w.RenderRequired(name, value, attrs, nil)
}

// RenderRequired implements MultiValueWidget.
func (w MultiWidget) RenderRequired(name string, value any, attrs Attrs, required []bool) string {
	return strings.Join(w.RenderSubwidgets(name, value, attrs, required), "")
}

// RenderSubwidgets returns the HTML of each subwidget, the subwidgets of the
// context of a Django MultiWidget, for a widget that lays them out itself.
// Subwidget i gets the attributes of the widget, its id with the suffix _<i>,
// and the required attribute required[i] when required is not nil.
func (w MultiWidget) RenderSubwidgets(name string, value any, attrs Attrs, required []bool) []string {
	values := w.SubValues(value)
	final := mergeAttrs(w.Attrs, attrs)
	id, _ := final["id"].(string)
	out := make([]string, len(w.Widgets))
	for i, sub := range w.Widgets {
		var v any
		if i < len(values) {
			v = values[i]
		}
		subAttrs := final
		if id != "" || required != nil {
			subAttrs = mergeAttrs(final)
		}
		if id != "" {
			subAttrs["id"] = id + "_" + strconv.Itoa(i)
		}
		if i < len(required) {
			subAttrs["required"] = required[i]
		}
		out[i] = sub.Render(name+w.suffix(i), v, subAttrs)
	}
	return out
}

// Attributes implements Widget.
func (w MultiWidget) Attributes() Attrs { return w.Attrs }

// ValueFromData returns the values of the subwidgets, a []any.
func (w MultiWidget) ValueFromData(data url.Values, name string) any {
	out := make([]any, len(w.Widgets))
	for i, sub := range w.Widgets {
		out[i] = sub.ValueFromData(data, name+w.suffix(i))
	}
	return out
}

// ValueOmittedFromData reports whether the data holds no value for any
// subwidget.
func (w MultiWidget) ValueOmittedFromData(data url.Values, name string) bool {
	for i, sub := range w.Widgets {
		if !sub.ValueOmittedFromData(data, name+w.suffix(i)) {
			return false
		}
	}
	return true
}

// IDForLabel returns "": the label points at no single subwidget.
func (MultiWidget) IDForLabel(string) string { return "" }

// UseRequiredAttribute allows the required attribute unless the widget is
// hidden.
func (w MultiWidget) UseRequiredAttribute(any) bool { return !w.IsHidden() }

// IsHidden reports whether every subwidget is hidden.
func (w MultiWidget) IsHidden() bool {
	for _, sub := range w.Widgets {
		if !sub.IsHidden() {
			return false
		}
	}
	return true
}

// UseFieldset reports true: the subwidgets belong in a <fieldset>.
func (MultiWidget) UseFieldset() bool { return true }

// Media returns the media of all subwidgets.
func (w MultiWidget) Media() Media {
	var m Media
	for _, sub := range w.Widgets {
		m = m.Add(WidgetMedia(sub))
	}
	return m
}

// asList returns value as a list when it is one.
func asList(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case []string:
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

// SplitDateTimeWidget renders a date and time as two text inputs, one for the
// date and one for the time, Django's SplitDateTimeWidget. It is the widget
// of a SplitDateTimeField; its subwidgets are named <name>_0 and <name>_1.
type SplitDateTimeWidget struct {
	widgetDefaults
	// Attrs are the HTML attributes of both inputs, unless DateAttrs or
	// TimeAttrs replace them.
	Attrs Attrs
	// DateAttrs are the HTML attributes of the date input.
	DateAttrs Attrs
	// TimeAttrs are the HTML attributes of the time input.
	TimeAttrs Attrs
	// DateFormat is the layout of the date, as for DateInput.Format.
	DateFormat string
	// TimeFormat is the layout of the time, as for TimeInput.Format.
	TimeFormat string
}

// multi returns the MultiWidget the widget renders as, with inputs of the
// given type.
func (w SplitDateTimeWidget) multi(inputType string) MultiWidget {
	dateAttrs, timeAttrs := w.Attrs, w.Attrs
	if w.DateAttrs != nil {
		dateAttrs = w.DateAttrs
	}
	if w.TimeAttrs != nil {
		timeAttrs = w.TimeAttrs
	}
	if inputType != "" {
		dateAttrs = mergeAttrs(dateAttrs, Attrs{"type": inputType})
		timeAttrs = mergeAttrs(timeAttrs, Attrs{"type": inputType})
	}
	return MultiWidget{
		Widgets: []Widget{
			DateInput{Attrs: dateAttrs, Format: w.DateFormat},
			TimeInput{Attrs: timeAttrs, Format: w.TimeFormat},
		},
		Decompress: decompressDateTime,
	}
}

// decompressDateTime splits a time.Time into its date and its time of day in
// UTC, the current time zone of the package, both the time.Time itself, which
// the date and time inputs format. Any other value gives two empty values.
func decompressDateTime(value any) []any {
	if t, ok := value.(time.Time); ok {
		t = t.UTC()
		return []any{t, t}
	}
	return []any{nil, nil}
}

// Subwidgets implements MultiValueWidget.
func (w SplitDateTimeWidget) Subwidgets() []Widget { return w.multi("").Widgets }

// SubValues implements MultiValueWidget.
func (w SplitDateTimeWidget) SubValues(value any) []any { return w.multi("").SubValues(value) }

// RenderRequired implements MultiValueWidget.
func (w SplitDateTimeWidget) RenderRequired(name string, value any, attrs Attrs, required []bool) string {
	return w.multi("").RenderRequired(name, value, attrs, required)
}

// Render renders the date and the time input.
func (w SplitDateTimeWidget) Render(name string, value any, attrs Attrs) string {
	return w.multi("").Render(name, value, attrs)
}

// Attributes returns nil: the attributes belong to the inputs.
func (SplitDateTimeWidget) Attributes() Attrs { return nil }

// ValueFromData returns the date and the time, a []any.
func (w SplitDateTimeWidget) ValueFromData(data url.Values, name string) any {
	return w.multi("").ValueFromData(data, name)
}

// ValueOmittedFromData reports whether the data holds neither a date nor a
// time.
func (w SplitDateTimeWidget) ValueOmittedFromData(data url.Values, name string) bool {
	return w.multi("").ValueOmittedFromData(data, name)
}

// IDForLabel returns "": the label points at neither input.
func (SplitDateTimeWidget) IDForLabel(string) string { return "" }

// UseFieldset reports true: the inputs belong in a <fieldset>.
func (SplitDateTimeWidget) UseFieldset() bool { return true }

// SupportsMicroseconds reports false: the time input shows whole seconds.
func (SplitDateTimeWidget) SupportsMicroseconds() bool { return false }

// SplitHiddenDateTimeWidget is SplitDateTimeWidget with hidden inputs,
// Django's SplitHiddenDateTimeWidget, the widget a SplitDateTimeField renders
// as hidden.
type SplitHiddenDateTimeWidget struct {
	widgetDefaults
	// Attrs are the HTML attributes of both inputs, unless DateAttrs or
	// TimeAttrs replace them.
	Attrs Attrs
	// DateAttrs are the HTML attributes of the date input.
	DateAttrs Attrs
	// TimeAttrs are the HTML attributes of the time input.
	TimeAttrs Attrs
	// DateFormat is the layout of the date, as for DateInput.Format.
	DateFormat string
	// TimeFormat is the layout of the time, as for TimeInput.Format.
	TimeFormat string
}

// multi returns the MultiWidget the widget renders as.
func (w SplitHiddenDateTimeWidget) multi() MultiWidget {
	return SplitDateTimeWidget(w).multi("hidden")
}

// Subwidgets implements MultiValueWidget.
func (w SplitHiddenDateTimeWidget) Subwidgets() []Widget { return w.multi().Widgets }

// SubValues implements MultiValueWidget.
func (w SplitHiddenDateTimeWidget) SubValues(value any) []any { return w.multi().SubValues(value) }

// RenderRequired implements MultiValueWidget.
func (w SplitHiddenDateTimeWidget) RenderRequired(name string, value any, attrs Attrs, required []bool) string {
	return w.multi().RenderRequired(name, value, attrs, required)
}

// Render renders the hidden date and time inputs.
func (w SplitHiddenDateTimeWidget) Render(name string, value any, attrs Attrs) string {
	return w.multi().Render(name, value, attrs)
}

// Attributes returns nil: the attributes belong to the inputs.
func (SplitHiddenDateTimeWidget) Attributes() Attrs { return nil }

// ValueFromData returns the date and the time, a []any.
func (w SplitHiddenDateTimeWidget) ValueFromData(data url.Values, name string) any {
	return w.multi().ValueFromData(data, name)
}

// ValueOmittedFromData reports whether the data holds neither a date nor a
// time.
func (w SplitHiddenDateTimeWidget) ValueOmittedFromData(data url.Values, name string) bool {
	return w.multi().ValueOmittedFromData(data, name)
}

// IDForLabel returns "": the label points at neither input.
func (SplitHiddenDateTimeWidget) IDForLabel(string) string { return "" }

// UseRequiredAttribute reports false: hidden inputs cannot be filled in.
func (SplitHiddenDateTimeWidget) UseRequiredAttribute(any) bool { return false }

// IsHidden reports true.
func (SplitHiddenDateTimeWidget) IsHidden() bool { return true }

// UseFieldset reports true, as for SplitDateTimeWidget.
func (SplitHiddenDateTimeWidget) UseFieldset() bool { return true }

// SupportsMicroseconds reports false: the time input shows whole seconds.
func (SplitHiddenDateTimeWidget) SupportsMicroseconds() bool { return false }
