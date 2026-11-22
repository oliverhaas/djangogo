// Package forms validates submitted data and renders HTML forms, following
// django.forms: the same field classes, validation steps, error messages and
// HTML output.
//
// # Declaring a form
//
// Django declares a form as a class; here a function builds it per request:
//
//	func newContactForm() *forms.Form {
//		return forms.New(
//			forms.F("subject", forms.NewCharField(forms.MaxLength(100))),
//			forms.F("message", forms.NewCharField(forms.UseWidget(forms.Textarea{}))),
//			forms.F("sender", forms.NewEmailField()),
//			forms.F("cc_myself", forms.NewBooleanField(forms.Required(false))),
//		)
//	}
//
// The keyword arguments of a Django field are FieldOption values (Required,
// Label, Initial, HelpText, UseWidget, ErrorMessages, Validators, MaxLength,
// ...). The constructor arguments of a Django form are exported fields of Form
// (Prefix, AutoID, Initial, LabelSuffix, ...).
//
// # Binding and validating
//
// Bind returns a bound copy, so the unbound form can be kept as a template:
//
//	form := newContactForm().Bind(r.PostForm)
//	if form.IsValid() {
//		data := form.CleanedData()
//	}
//
// A field cleans a value in Django's three steps: ToGo (to_python), Validate
// and RunValidators. The cleaned values are Go values:
//
//	CharField, EmailField, URLField, SlugField, RegexField  string
//	IntegerField                                             int64
//	FloatField                                               float64
//	DecimalField                                             forms.Decimal
//	BooleanField                                             bool
//	NullBooleanField                                         bool or nil
//	DateField, TimeField, DateTimeField                      time.Time
//	DurationField                                            time.Duration
//	UUIDField                                                canonical string
//	ChoiceField                                              string
//	TypedChoiceField                                         result of Coerce
//	MultipleChoiceField                                      []string
//	ModelChoiceField                                         *T
//
// An empty optional value is nil, except for the text fields ("") and the
// boolean field (false).
//
// A clean_<name>() method is a function registered with Form.CleanFieldWith,
// clean() one registered with Form.CleanWith; both report errors by returning
// a *ValidationError or by calling Form.AddError. Form.Errors returns an
// ErrorDict with Django's AsData, AsJSON, AsText and AsUL.
//
// # Rendering
//
// A form renders with AsDiv (the default, also String), AsP, AsTable and AsUL;
// Form.Field returns a BoundField with LabelTag, Errors, IDForLabel and the
// widget HTML. The HTML is Django's, with two differences that carry no
// meaning: the white space between tags, and the order of attributes, which
// is alphabetical followed by required, disabled, aria-invalid,
// aria-describedby, id, multiple, checked and selected.
//
// In a template a form is used as in Django:
//
//	{{ form }}  {{ form.as_p }}  {{ form.non_field_errors }}
//	{{ form.title }}  {{ form.title.label_tag }}  {{ form.title.errors }}
//	{% for field in form %}{{ field.label_tag }} {{ field }}{% endfor %}
//	{% for radio in form.language %}{{ radio.tag }} {{ radio.choice_label }}{% endfor %}
//
// views.Render converts a form in the context through TemplateValue. A loop
// over a field yields its subwidgets, see BoundField.Subwidgets.
//
// A Choice with options in Group is an option group, Django's nested
// choices. A Select renders it as an <optgroup>, a RadioSelect and a
// CheckboxSelectMultiple as a <div> under the name of the group. Subwidgets
// returns the options without their groups, as Django's options() does.
//
// # Model forms
//
// NewModelForm derives a form from a registered model, validates unique
// fields against the database and saves the instance:
//
//	form := forms.NewModelForm[Post](db, forms.WithFields("title", "slug")).Bind(r.PostForm)
//	form.SetContext(r.Context())
//	if form.IsValid() {
//		post, err := form.Save(r.Context())
//	}
//
// The Meta options of a Django ModelForm are ModelFormOption values. A field
// is named after the attribute of its model field ("created_at"), is required
// unless the model field allows blank, and takes label, help text, choices,
// maximum length and default from it. A foreign key becomes a
// ModelChoiceField over the rows of the related model. A new instance starts
// with the defaults of the model, and a field with a default that the data
// leaves out keeps it, as in Django's construct_instance. A name in WithFields
// that the model does not have makes the form fail (Form.Err) with Django's
// FieldError message.
//
// # Formsets
//
// A FormSet shows and validates several copies of a form, as Django's
// formset_factory does. The arguments of formset_factory are exported fields:
//
//	fs := forms.NewFormSet(newArticleForm)
//	fs.Extra, fs.CanDelete = 2, true
//	bound := fs.Bind(r.PostForm)
//	if bound.IsValid() {
//		for _, data := range bound.CleanedData() { ... }
//	}
//
// ConfigureForm changes each form after it is built, where Django passes
// form_kwargs. A clean() method of the formset is a function registered with
// FormSet.CleanWith; its errors are the non-form errors. In a template a
// formset is used as in Django: {{ formset.management_form }}, {% for form in
// formset %} and {{ formset.empty_form }}.
//
// # Model formsets
//
// A ModelFormSet edits the rows of a model, as Django's modelformset_factory
// does, an InlineFormSet edits the rows that point at one parent row, as
// inlineformset_factory does, and a GenericInlineFormSet edits the rows that
// point at one object of any model through a generic foreign key, as
// generic_inlineformset_factory of django.contrib.contenttypes.forms does:
//
//	fs := forms.NewInlineFormSet[Author, Book](db, "", forms.WithFields("title"))
//	bound := fs.WithInstance(author).Bind(r.PostForm)
//	bound.SetContext(r.Context())
//	if bound.IsValid() {
//		books, err := bound.Save(r.Context())
//	}
//
// The ModelFormOption values build each form as NewModelForm does. QuerySet
// selects the rows of a model formset; each form carries the primary key of
// its row in a hidden field. Validation also checks the unique fields across
// the forms. Save adds, changes and deletes rows, and SaveCommit with commit
// false returns the instances without saving them. The SaveNew, SaveExisting
// and DeleteExisting hooks take the place of the methods that a Django
// formset overrides.
//
// # Not covered
//
// File uploads (FileField, ImageField, ClearableFileInput) and localized
// input formats are not part of this package.
package forms
