package admin

import (
	"cmp"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// formfieldCallback returns the form field callback of the forms of e for r,
// formfieldForDBField. It adds the attribute name of each field it builds to
// built.
func (e *entry) formfieldCallback(r *http.Request, built map[string]bool) func(mf *orm.Field, f forms.Field) forms.Field {
	autocomplete := e.autocompleteFields(r)
	return func(mf *orm.Field, f forms.Field) forms.Field {
		built[mf.Attr] = true
		return e.formfieldForDBField(mf, f, autocomplete)
	}
}

// formfieldForDBField gives f, the form field of mf, the admin's widget,
// Django's ModelAdmin.formfield_for_dbfield. A field with choices or a
// foreign key of RadioFields gets radio buttons, unless the foreign key is an
// autocomplete field or a raw-id field or FormfieldOverrides gives it a
// widget. A foreign key takes the empty label of FormfieldOverrides. Any
// other field takes the widget of formfieldOverride, and a date and time
// field becomes a SplitDateTimeField unless its entry has a FormClass. The
// FormClass and the Options of an entry apply through overrideOptions,
// before the field is built. relationWidgets sets the other widgets
// of the relations. ModelAdmin.FormOptions apply after it, as the Meta
// options of a Django ModelAdmin.form win over the defaults.
func (e *entry) formfieldForDBField(mf *orm.Field, f forms.Field, autocomplete []string) forms.Field {
	style, ok := e.admin.RadioFields[mf.Attr]
	if !ok {
		style = e.admin.RadioFields[mf.Name]
	}
	class, o := e.formfieldOverride(mf)
	switch {
	case mf.HasChoices:
		if style != 0 {
			return radioField(mf, f, style, nil)
		}
	case mf.Rel != nil:
		if style != 0 && o.Widget == nil && !namesField(autocomplete, mf) && !namesField(e.admin.RawIDFields, mf) {
			return radioField(mf, f, style, o.EmptyLabel)
		}
		if field, ok := f.(*forms.ModelChoiceField); ok && o.EmptyLabel != nil {
			field.EmptyLabel = o.EmptyLabel
		}
	case class == DateTimeField && o.FormClass == nil:
		return splitDateTimeField(f, o.Widget)
	case o.Widget != nil:
		f.Base().Widget = o.Widget
	}
	return f
}

// overrideOptions returns the form options that give the model fields of e
// the FormClass and the Options of their FormfieldOverrides entry, as
// Django's formfield_for_dbfield passes the entry to formfield(). A field
// with choices takes none, as in Django, and a relation keeps its choice
// field. Pass them before the form options of the admin, which win, as the
// Meta options of a Django ModelAdmin.form do.
func (e *entry) overrideOptions() []forms.ModelFormOption {
	var out []forms.ModelFormOption
	for _, mf := range slices.Concat(e.model.Fields(), e.model.ManyToMany()) {
		if mf.HasChoices {
			continue
		}
		_, o := e.formfieldOverride(mf)
		if len(o.Options) > 0 {
			out = append(out, forms.WithFieldOptions(mf.Attr, o.Options...))
		}
		if o.FormClass != nil && mf.Rel == nil {
			out = append(out, forms.WithFieldClass(mf.Attr, o.FormClass))
		}
	}
	return out
}

// formfieldOverride returns the class of mf whose FormfieldOverrides entry
// or default widget applies to mf, with the entry merged over the default,
// as Django's formfield_for_dbfield walks the mro of the model field class.
// A relation has its own class alone and no default. class is 0 when no
// class of mf has an entry or a default.
func (e *entry) formfieldOverride(mf *orm.Field) (FieldClass, FormfieldOverride) {
	classes := fieldClasses(mf)
	if mf.Rel != nil {
		return classes[0], e.admin.FormfieldOverrides[classes[0]]
	}
	for _, class := range classes {
		o, ok := e.admin.FormfieldOverrides[class]
		def := defaultWidget(class)
		if !ok && def == nil {
			continue
		}
		if o.Widget == nil {
			o.Widget = def
		}
		return class, o
	}
	return 0, FormfieldOverride{}
}

// radioField gives f, the form field of mf, the radio buttons of style,
// Django's formfield_for_choice_field and formfield_for_foreignkey for a
// field of RadioFields. Django's AdminRadioSelect renders as a RadioSelect.
// Only a field that can be blank has the empty choice. Its label is "None",
// or emptyLabel for a foreign key with one. A field that is neither a
// foreign key nor a field with choices keeps f.
func radioField(mf *orm.Field, f forms.Field, style RadioStyle, emptyLabel *string) forms.Field {
	switch field := f.(type) {
	case *forms.TypedChoiceField:
		field.Choices = radioChoices(mf)
	case *forms.ModelChoiceField:
		field.EmptyLabel = nil
		if mf.Blank {
			field.EmptyLabel = cmp.Or(emptyLabel, new("None"))
		}
	default:
		return f
	}
	f.Base().Widget = forms.RadioSelect{Attrs: forms.Attrs{"class": radioClass(style)}}
	return f
}

// radioChoices returns the choices of mf as radio buttons, Django's
// get_choices(include_blank=db_field.blank, blank_choice=[("", "None")]): the
// empty choice "None" first for a field that can be blank and has no empty
// choice of its own.
func radioChoices(mf *orm.Field) []forms.Choice {
	var out []forms.Choice
	if mf.Blank && len(mf.GetChoices(true)) > len(mf.GetChoices(false)) {
		out = append(out, forms.Choice{Value: "", Label: "None"})
	}
	for _, c := range mf.FlatChoices() {
		out = append(out, forms.Choice{Value: c.Value, Label: c.Label})
	}
	return out
}

// radioClass returns the class of the radio buttons of style, Django's
// get_ul_class.
func radioClass(style RadioStyle) string {
	if style == Vertical {
		return "radiolist"
	}
	return "radiolist inline"
}

// splitDateTimeField returns the admin's field for a date and time model
// field, which Django's defaults give the form class SplitDateTimeField: a
// SplitDateTimeField with the options of the model form's field f and the
// widget w.
func splitDateTimeField(f forms.Field, w forms.Widget) forms.Field {
	b := f.Base()
	split := forms.NewSplitDateTimeField(forms.Required(b.Required), forms.Initial(b.Initial),
		forms.HelpText(b.HelpText), forms.Validators(b.Validators...), forms.UseWidget(w))
	split.Label, split.LabelSuffix = b.Label, b.LabelSuffix
	return split
}

// kindClasses maps each kind of a model field that is not a relation to the
// Django class of the kind and the classes it derives from, most specific
// first, without Field.
var kindClasses = map[orm.Kind][]FieldClass{
	orm.KindBool:             {BooleanField},
	orm.KindChar:             {CharField},
	orm.KindSlug:             {SlugField, CharField},
	orm.KindEmail:            {EmailField, CharField},
	orm.KindURL:              {URLField, CharField},
	orm.KindText:             {TextField},
	orm.KindDate:             {DateField},
	orm.KindDateTime:         {DateTimeField, DateField},
	orm.KindTime:             {TimeField},
	orm.KindDuration:         {DurationField},
	orm.KindDecimal:          {DecimalField},
	orm.KindFloat:            {FloatField},
	orm.KindInt:              {IntegerField},
	orm.KindSmallInt:         {SmallIntegerField, IntegerField},
	orm.KindBigInt:           {BigIntegerField, IntegerField},
	orm.KindPositiveInt:      {PositiveIntegerField, IntegerField},
	orm.KindPositiveSmallInt: {PositiveSmallIntegerField, SmallIntegerField, IntegerField},
	orm.KindPositiveBigInt:   {PositiveBigIntegerField, BigIntegerField, IntegerField},
	orm.KindUUID:             {UUIDField},
	orm.KindJSON:             {JSONField},
	orm.KindBinary:           {BinaryField},
	orm.KindGenericIPAddress: {GenericIPAddressField},
}

// fieldClasses returns the classes of mf, most specific first, as the mro of
// its Django class: ForeignKey, OneToOneField or ManyToManyField for a
// relation, else the classes of its kind and Field.
func fieldClasses(mf *orm.Field) []FieldClass {
	if mf.Rel != nil {
		switch mf.Rel.Kind {
		case orm.RelO2O:
			return []FieldClass{OneToOneField}
		case orm.RelM2M:
			return []FieldClass{ManyToManyField}
		}
		return []FieldClass{ForeignKey}
	}
	return append(slices.Clone(kindClasses[mf.Kind]), Field)
}

// defaultWidget returns the admin's widget for the model fields of class,
// Django's FORMFIELD_FOR_DBFIELD_DEFAULTS, or nil for a class without one.
func defaultWidget(class FieldClass) forms.Widget {
	switch class {
	case DateTimeField:
		return NewAdminSplitDateTime(nil)
	case DateField:
		return NewAdminDateWidget(nil)
	case TimeField:
		return NewAdminTimeWidget(nil)
	case TextField:
		return NewAdminTextareaWidget(nil)
	case URLField:
		return NewAdminURLFieldWidget(nil)
	case IntegerField:
		return NewAdminIntegerFieldWidget(nil)
	case BigIntegerField:
		return NewAdminBigIntegerFieldWidget(nil)
	case CharField:
		return NewAdminTextInputWidget(nil)
	case EmailField:
		return NewAdminEmailInputWidget(nil)
	case UUIDField:
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
