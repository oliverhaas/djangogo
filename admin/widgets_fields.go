package admin

import (
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// formfieldForDBField is the form field callback of the add and change forms,
// the defaults of Django's ModelAdmin.formfield_for_dbfield
// (FORMFIELD_FOR_DBFIELD_DEFAULTS): the admin's widget for each model field
// that is not a relation and has no choices. ModelAdmin.FormOptions apply
// after it, as the Meta options of a Django ModelAdmin.form win over the
// defaults.
func formfieldForDBField(mf *orm.Field, f forms.Field) forms.Field {
	if mf.Rel != nil || mf.HasChoices {
		return f
	}
	if mf.Kind == orm.KindDateTime {
		return splitDateTimeField(f)
	}
	if w := dbFieldWidget(mf); w != nil {
		f.Base().Widget = w
	}
	return f
}

// splitDateTimeField returns the admin's field for a date and time model
// field, which Django's defaults give the form class SplitDateTimeField and
// the widget AdminSplitDateTime: a SplitDateTimeField with the options of
// the model form's field f.
func splitDateTimeField(f forms.Field) forms.Field {
	b := f.Base()
	split := forms.NewSplitDateTimeField(forms.Required(b.Required), forms.Initial(b.Initial),
		forms.HelpText(b.HelpText), forms.Validators(b.Validators...), forms.UseWidget(NewAdminSplitDateTime(nil)))
	split.Label, split.LabelSuffix = b.Label, b.LabelSuffix
	return split
}

// dbFieldWidget returns the admin's widget for the kind of mf, nil for a kind
// that keeps the widget of its form field.
func dbFieldWidget(mf *orm.Field) forms.Widget {
	switch mf.Kind {
	case orm.KindDate:
		return NewAdminDateWidget(nil)
	case orm.KindTime:
		return NewAdminTimeWidget(nil)
	case orm.KindText:
		return NewAdminTextareaWidget(nil)
	case orm.KindURL:
		return NewAdminURLFieldWidget(nil)
	case orm.KindInt, orm.KindSmallInt, orm.KindPositiveInt, orm.KindPositiveSmallInt:
		return NewAdminIntegerFieldWidget(nil)
	case orm.KindBigInt, orm.KindPositiveBigInt:
		return NewAdminBigIntegerFieldWidget(nil)
	case orm.KindChar, orm.KindSlug:
		return NewAdminTextInputWidget(nil)
	case orm.KindEmail:
		return NewAdminEmailInputWidget(nil)
	case orm.KindUUID:
		return NewAdminUUIDInputWidget(nil)
	}
	return nil
}

// classAttrs returns attrs over the given class, the attributes of the admin's
// input widgets.
func classAttrs(class string, attrs forms.Attrs) forms.Attrs {
	return mergeWidgetAttrs(forms.Attrs{"class": class}, attrs)
}

// NewAdminTextInputWidget returns the widget of a character field in the
// admin, Django's AdminTextInputWidget: a text input of class vTextField,
// with attrs over that class.
func NewAdminTextInputWidget(attrs forms.Attrs) forms.TextInput {
	return forms.TextInput{Attrs: classAttrs("vTextField", attrs)}
}

// NewAdminTextareaWidget returns the widget of a text field in the admin,
// Django's AdminTextareaWidget: a textarea of class vLargeTextField.
func NewAdminTextareaWidget(attrs forms.Attrs) forms.Textarea {
	return forms.Textarea{Attrs: classAttrs("vLargeTextField", attrs)}
}

// NewAdminEmailInputWidget returns the widget of an email field in the admin,
// Django's AdminEmailInputWidget: an email input of class vTextField.
func NewAdminEmailInputWidget(attrs forms.Attrs) forms.EmailInput {
	return forms.EmailInput{Attrs: classAttrs("vTextField", attrs)}
}

// NewAdminIntegerFieldWidget returns the widget of an integer field in the
// admin, Django's AdminIntegerFieldWidget: a number input of class
// vIntegerField.
func NewAdminIntegerFieldWidget(attrs forms.Attrs) forms.NumberInput {
	return forms.NumberInput{Attrs: classAttrs("vIntegerField", attrs)}
}

// NewAdminBigIntegerFieldWidget returns the widget of a big integer field in
// the admin, Django's AdminBigIntegerFieldWidget: a number input of class
// vBigIntegerField.
func NewAdminBigIntegerFieldWidget(attrs forms.Attrs) forms.NumberInput {
	return forms.NumberInput{Attrs: classAttrs("vBigIntegerField", attrs)}
}

// NewAdminUUIDInputWidget returns the widget of a UUID field in the admin,
// Django's AdminUUIDInputWidget: a text input of class vUUIDField.
func NewAdminUUIDInputWidget(attrs forms.Attrs) forms.TextInput {
	return forms.TextInput{Attrs: classAttrs("vUUIDField", attrs)}
}

// timezoneWarningID returns the id of the warning that Django's
// DateTimeShortcuts.js shows below a date or time input when the browser's
// time zone differs from the server's.
func timezoneWarningID(name string) string {
	return "id_" + name + "_timezone_warning_helptext"
}

// dateTimeMedia returns the scripts of the calendar and the clock, the media
// of Django's BaseAdminDateWidget and BaseAdminTimeWidget.
func dateTimeMedia() forms.Media {
	return adminJS("calendar.js", "admin/DateTimeShortcuts.js")
}

// AdminDateWidget is the widget of a date field in the admin, Django's
// admin.widgets.AdminDateWidget: a text input of class vDateField, to which
// DateTimeShortcuts.js adds a calendar and a "Today" link.
//
//nolint:revive // AdminDateWidget is the intended public name, matching Django.
type AdminDateWidget struct {
	forms.DateInput
}

// NewAdminDateWidget returns the date widget with attrs over its class
// vDateField and size 10.
func NewAdminDateWidget(attrs forms.Attrs) AdminDateWidget {
	return AdminDateWidget{forms.DateInput{Attrs: mergeWidgetAttrs(forms.Attrs{"class": "vDateField", "size": "10"}, attrs)}}
}

// Render implements forms.Widget: the input in a <p class="date">, described
// by the time zone warning.
func (w AdminDateWidget) Render(name string, value any, attrs forms.Attrs) string {
	attrs = mergeWidgetAttrs(attrs, forms.Attrs{"aria-describedby": timezoneWarningID(name)})
	return "<p class=\"date\">\n  " + w.DateInput.Render(name, value, attrs) + "\n</p>"
}

// Media returns the scripts of the calendar.
func (AdminDateWidget) Media() forms.Media { return dateTimeMedia() }

// AdminTimeWidget is the widget of a time field in the admin, Django's
// admin.widgets.AdminTimeWidget: a text input of class vTimeField, to which
// DateTimeShortcuts.js adds a clock and a "Now" link.
//
//nolint:revive // AdminTimeWidget is the intended public name, matching Django.
type AdminTimeWidget struct {
	forms.TimeInput
}

// NewAdminTimeWidget returns the time widget with attrs over its class
// vTimeField and size 8.
func NewAdminTimeWidget(attrs forms.Attrs) AdminTimeWidget {
	return AdminTimeWidget{forms.TimeInput{Attrs: mergeWidgetAttrs(forms.Attrs{"class": "vTimeField", "size": "8"}, attrs)}}
}

// Render implements forms.Widget: the input in a <p class="time">, described
// by the time zone warning.
func (w AdminTimeWidget) Render(name string, value any, attrs forms.Attrs) string {
	attrs = mergeWidgetAttrs(attrs, forms.Attrs{"aria-describedby": timezoneWarningID(name)})
	return "<p class=\"time\">\n  " + w.TimeInput.Render(name, value, attrs) + "\n</p>"
}

// Media returns the scripts of the clock.
func (AdminTimeWidget) Media() forms.Media { return dateTimeMedia() }

// AdminSplitDateTime is the widget of a date and time field in the admin,
// Django's admin.widgets.AdminSplitDateTime: a date input of class vDateField
// and a time input of class vTimeField, each with a label, to which
// DateTimeShortcuts.js adds a calendar and a clock. Its inputs are named
// <name>_0 and <name>_1, as those of forms.SplitDateTimeWidget.
//
//nolint:revive // AdminSplitDateTime is the intended public name, matching Django.
type AdminSplitDateTime struct {
	// Attrs are the HTML attributes of both inputs, over their class and
	// size.
	Attrs forms.Attrs
}

// NewAdminSplitDateTime returns the split date and time widget with attrs on
// both inputs.
func NewAdminSplitDateTime(attrs forms.Attrs) AdminSplitDateTime {
	return AdminSplitDateTime{Attrs: attrs}
}

// multi returns the date and the time input as a MultiWidget.
func (w AdminSplitDateTime) multi() forms.MultiWidget {
	return forms.MultiWidget{
		Widgets: []forms.Widget{
			forms.DateInput{Attrs: forms.Attrs{"class": "vDateField", "size": "10"}},
			forms.TimeInput{Attrs: forms.Attrs{"class": "vTimeField", "size": "8"}},
		},
		Attrs:      w.Attrs,
		Decompress: forms.SplitDateTimeWidget{}.SubValues,
	}
}

// Render implements forms.Widget, Django's admin/widgets/split_datetime.html.
func (w AdminSplitDateTime) Render(name string, value any, attrs forms.Attrs) string {
	return w.RenderRequired(name, value, attrs, nil)
}

// RenderRequired implements forms.MultiValueWidget: the labeled inputs in a
// <p class="datetime">, both described by the time zone warning.
func (w AdminSplitDateTime) RenderRequired(name string, value any, attrs forms.Attrs, required []bool) string {
	attrs = mergeWidgetAttrs(attrs, forms.Attrs{"aria-describedby": timezoneWarningID(name)})
	subs := w.multi().RenderSubwidgets(name, value, attrs, required)
	id, _ := mergeWidgetAttrs(w.Attrs, attrs)["id"].(string)
	label := func(i int, text string) string {
		if id == "" {
			return "<label>" + text + "</label>"
		}
		return fmt.Sprintf(`<label for="%s_%d">%s</label>`, html.EscapeString(id), i, text)
	}
	return "<p class=\"datetime\">\n  " + label(0, "Date:") + " " + subs[0] + "<br>\n  " +
		label(1, "Time:") + " " + subs[1] + "\n</p>"
}

// Subwidgets implements forms.MultiValueWidget.
func (w AdminSplitDateTime) Subwidgets() []forms.Widget { return w.multi().Widgets }

// SubValues implements forms.MultiValueWidget.
func (w AdminSplitDateTime) SubValues(value any) []any { return w.multi().SubValues(value) }

// Attributes implements forms.Widget.
func (w AdminSplitDateTime) Attributes() forms.Attrs { return w.Attrs }

// ValueFromData returns the date and the time, a []any.
func (w AdminSplitDateTime) ValueFromData(data url.Values, name string) any {
	return w.multi().ValueFromData(data, name)
}

// ValueOmittedFromData reports whether the data holds neither a date nor a
// time.
func (w AdminSplitDateTime) ValueOmittedFromData(data url.Values, name string) bool {
	return w.multi().ValueOmittedFromData(data, name)
}

// IDForLabel returns id unchanged, as Django's AdminSplitDateTime does.
func (AdminSplitDateTime) IDForLabel(id string) string { return id }

// UseRequiredAttribute reports true: the inputs can be filled in.
func (AdminSplitDateTime) UseRequiredAttribute(any) bool { return true }

// IsHidden reports false.
func (AdminSplitDateTime) IsHidden() bool { return false }

// UseFieldset reports true: the inputs belong in a <fieldset>.
func (AdminSplitDateTime) UseFieldset() bool { return true }

// SupportsMicroseconds reports false: the time input shows whole seconds.
func (AdminSplitDateTime) SupportsMicroseconds() bool { return false }

// Media returns the scripts of the calendar and the clock.
func (AdminSplitDateTime) Media() forms.Media { return dateTimeMedia() }

// AdminURLFieldWidget is the widget of a URL field in the admin, Django's
// admin.widgets.AdminURLFieldWidget: a URL input of class vURLField that
// shows a valid current URL as a link above it.
//
//nolint:revive // AdminURLFieldWidget is the intended public name, matching Django.
type AdminURLFieldWidget struct {
	forms.URLInput
}

// NewAdminURLFieldWidget returns the URL widget with attrs over its class
// vURLField.
func NewAdminURLFieldWidget(attrs forms.Attrs) AdminURLFieldWidget {
	return AdminURLFieldWidget{forms.URLInput{Attrs: classAttrs("vURLField", attrs)}}
}

// Render implements forms.Widget, Django's admin/widgets/url.html: a value
// that is a valid URL comes first as a "Currently:" link, and the input
// follows as "Change:".
func (w AdminURLFieldWidget) Render(name string, value any, attrs forms.Attrs) string {
	input := w.URLInput.Render(name, value, attrs)
	text := keyText(value)
	if forms.ValidateURL(text) != nil {
		return input
	}
	return `<p class="url">Currently: <a href="` + forms.Escape(smartURLQuote(text)) + `">` +
		forms.Escape(text) + "</a><br>Change: " + input + "</p>"
}

// urlQuoteSafe holds the characters that Django's smart_urlquote leaves as
// they are besides letters, digits and "_.-~": the sub-delims and gen-delims
// of RFC 3986.
const urlQuoteSafe = "!$&'()*+,;=:/?#[]@"

// smartURLQuote percent-encodes the parts of a URL that are not encoded yet,
// Django's django.utils.html.smart_urlquote. A non-ASCII host is encoded as
// UTF-8 octets, not with IDNA.
func smartURLQuote(u string) string {
	unquoteQuote := func(segment string) string { return pyQuote(pyUnquote(segment), urlQuoteSafe) }
	scheme, netloc, path, query, fragment, ok := urlSplit(u)
	if !ok {
		return unquoteQuote(u)
	}
	netloc = unquoteQuote(netloc)
	if query != "" {
		// Keys and values are unquoted one by one, so that an encoded "&" or
		// "=" in a value stays part of it.
		var parts []string
		for pair := range strings.SplitSeq(query, "&") {
			if pair == "" {
				continue
			}
			k, v, _ := strings.Cut(pair, "=")
			k = pyUnquote(pyUnquote(strings.ReplaceAll(k, "+", " ")))
			v = pyUnquote(pyUnquote(strings.ReplaceAll(v, "+", " ")))
			parts = append(parts, pyQuotePlus(k)+"="+pyQuotePlus(v))
		}
		query = strings.Join(parts, "&")
	}
	return urlUnsplit(scheme, netloc, unquoteQuote(path), query, unquoteQuote(fragment))
}

// urlSplit splits a URL into its five parts as Python's
// urllib.parse.urlsplit does; ok is false where urlsplit raises ValueError,
// for a bracket of an IPv6 host without its partner.
func urlSplit(u string) (scheme, netloc, path, query, fragment string, ok bool) {
	u = strings.TrimLeftFunc(u, func(r rune) bool { return r <= ' ' })
	u = strings.NewReplacer("\t", "", "\r", "", "\n", "").Replace(u)
	if i := strings.IndexByte(u, ':'); i > 0 && isASCIILetter(u[0]) && isSchemeText(u[:i]) {
		scheme, u = strings.ToLower(u[:i]), u[i+1:]
	}
	if strings.HasPrefix(u, "//") {
		end := len(u)
		if i := strings.IndexAny(u[2:], "/?#"); i >= 0 {
			end = i + 2
		}
		netloc, u = u[2:end], u[end:]
		if strings.Contains(netloc, "[") != strings.Contains(netloc, "]") {
			return "", "", "", "", "", false
		}
	}
	u, fragment, _ = strings.Cut(u, "#")
	u, query, _ = strings.Cut(u, "?")
	return scheme, netloc, u, query, fragment, true
}

// urlUnsplit joins the five parts of a URL as Python's
// urllib.parse.urlunsplit does.
func urlUnsplit(scheme, netloc, path, query, fragment string) string {
	u := path
	if netloc != "" {
		if u != "" && u[0] != '/' {
			u = "/" + u
		}
		u = "//" + netloc + u
	} else if strings.HasPrefix(u, "//") {
		u = "//" + u
	}
	if scheme != "" {
		u = scheme + ":" + u
	}
	if query != "" {
		u += "?" + query
	}
	if fragment != "" {
		u += "#" + fragment
	}
	return u
}

// isASCIILetter reports whether c is an ASCII letter.
func isASCIILetter(c byte) bool { return c|0x20 >= 'a' && c|0x20 <= 'z' }

// isSchemeText reports whether s holds only the characters of a URL scheme.
func isSchemeText(s string) bool {
	for i := range len(s) {
		c := s[i]
		if !isASCIILetter(c) && (c < '0' || c > '9') && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

// pyQuote percent-encodes the UTF-8 bytes of s as Python's
// urllib.parse.quote does: letters, digits, "_.-~" and the characters of safe
// stay as they are.
func pyQuote(s, safe string) string {
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		if c < 0x80 && (isASCIILetter(c) || (c >= '0' && c <= '9') ||
			strings.IndexByte("_.-~", c) >= 0 || strings.IndexByte(safe, c) >= 0) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// pyQuotePlus percent-encodes s as Python's urllib.parse.quote_plus does for a
// query string: as pyQuote without safe characters, with a space as "+".
func pyQuotePlus(s string) string {
	return strings.ReplaceAll(pyQuote(s, " "), " ", "+")
}

// pyUnquote decodes the %XX escapes of s as Python's urllib.parse.unquote
// does: an invalid escape stays as it is, and bytes that are not UTF-8 become
// U+FFFD.
func pyUnquote(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	buf := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			buf = append(buf, unhex(s[i+1])<<4|unhex(s[i+2]))
			i += 2
			continue
		}
		buf = append(buf, s[i])
	}
	return strings.ToValidUTF8(string(buf), "�")
}

// isHex reports whether c is a hexadecimal digit.
func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c|0x20 >= 'a' && c|0x20 <= 'f')
}

// unhex returns the value of the hexadecimal digit c.
func unhex(c byte) byte {
	if c <= '9' {
		return c - '0'
	}
	return c | 0x20 - 'a' + 10
}
