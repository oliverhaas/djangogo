package orm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/oliverhaas/djangogo/validators"
)

// NonFieldErrors is the key under which validation error dictionaries store
// model-wide errors (such as Clean() errors and unique_together violations),
// matching Django's NON_FIELD_ERRORS.
const NonFieldErrors = validators.NonFieldErrors

// ValidationError is a validation failure, aliased to
// validators.ValidationError so ORM, forms, and standalone validators share a
// single error type.
type ValidationError = validators.ValidationError

// Validator checks a converted field value, aliased to validators.Validator so
// validators from the validators or forms packages plug directly into
// Meta.Validators.
type Validator = validators.Validator

// NewValidationError returns a single ValidationError with the given message
// template, code, and optional params map.
func NewValidationError(message, code string, params ...map[string]any) *ValidationError {
	return validators.NewValidationError(message, code, params...)
}

// ValidationErrors returns a list ValidationError containing errs.
func ValidationErrors(errs ...any) *ValidationError {
	return validators.ValidationErrors(errs...)
}

// ValidationErrorDict returns a dictionary ValidationError mapping field names
// to errors.
func ValidationErrorDict(fields map[string]any) *ValidationError {
	return validators.ValidationErrorDict(fields)
}

// modelCleaner is implemented by models that define a custom model-wide Clean()
// hook (Django's Model.clean()).
type modelCleaner interface {
	Clean() error
}

// defaultFieldErrorMessages holds Django's verbatim Field.default_error_messages
// and ForeignKey invalid error message.
var defaultFieldErrorMessages = map[string]string{
	"invalid_choice":  "Value %(value)r is not a valid choice.",
	"null":            "This field cannot be null.",
	"blank":           "This field cannot be blank.",
	"unique":          "%(model_name)s with this %(field_label)s already exists.",
	"unique_together": "%(model_name)s with this %(field_labels)s already exists.",
	"unique_for_date": "%(field_label)s must be unique for %(date_field_label)s %(lookup_type)s.",
	"fk_invalid":      "%(model)s instance with %(field)s %(value)r is not a valid choice.",
}

// CleanOption configures FullClean, CleanFields, and ValidateUnique.
type CleanOption func(*cleanConfig)

// cleanConfig stores parsed options for model validation.
type cleanConfig struct {
	exclude             []string
	validateUnique      bool
	validateConstraints bool
	hasAdding           bool
	adding              bool
}

// defaultCleanConfig returns cleanConfig with Django's defaults
// (validate_unique=True, validate_constraints=True, no exclusions).
func defaultCleanConfig(opts []CleanOption) cleanConfig {
	cfg := cleanConfig{validateUnique: true, validateConstraints: true}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// WithExclude omits the named fields (Go struct names, attribute names, or
// column names) from validation, matching Django's exclude= argument. The
// caller's slice is never mutated.
func WithExclude(fields ...string) CleanOption {
	cp := append([]string(nil), fields...)
	return func(c *cleanConfig) {
		c.exclude = append(c.exclude, cp...)
	}
}

// WithValidateUnique enables or disables database uniqueness checks during
// FullClean, matching Django's validate_unique= argument (default true).
func WithValidateUnique(enabled bool) CleanOption {
	return func(c *cleanConfig) {
		c.validateUnique = enabled
	}
}

// WithValidateConstraints enables or disables the checks of Meta.Constraints
// during FullClean, matching Django's validate_constraints= argument (default
// true).
func WithValidateConstraints(enabled bool) CleanOption {
	return func(c *cleanConfig) {
		c.validateConstraints = enabled
	}
}

// WithAdding tells ValidateUnique whether the instance represents a new row
// being inserted (true) or an existing row being updated (false), matching
// Django's instance._state.adding. When omitted, ValidateUnique treats an
// instance with a zero primary key as adding and a non-zero primary key as
// existing.
func WithAdding(adding bool) CleanOption {
	return func(c *cleanConfig) {
		c.hasAdding = true
		c.adding = adding
	}
}

// ErrorMessage returns the message template for code on f, checking custom
// Meta.ErrorMessages overrides before Django's defaults.
func (f *Field) ErrorMessage(code string) string {
	if f.ErrorMessages != nil {
		if msg, ok := f.ErrorMessages[code]; ok {
			return msg
		}
	}
	if code == "invalid" && f.Rel != nil {
		return defaultFieldErrorMessages["fk_invalid"]
	}
	return defaultFieldErrorMessages[code]
}

// DefaultValidators returns the built-in validators implied by f's Kind and
// options (max_length, integer range, Decimal digits, Email, URL, Slug, IP
// protocol), matching Django's Field.default_validators and dynamic validators.
func (f *Field) DefaultValidators() []Validator {
	var out []Validator
	switch f.Kind {
	case KindChar:
		if f.MaxLength > 0 {
			out = append(out, validators.MaxLengthValidator(f.MaxLength))
		}
	case KindSlug:
		if f.AllowUnicode {
			out = append(out, validators.ValidateUnicodeSlug)
		} else {
			out = append(out, validators.ValidateSlug)
		}
		if f.MaxLength > 0 {
			out = append(out, validators.MaxLengthValidator(f.MaxLength))
		}
	case KindEmail:
		out = append(out, validators.ValidateEmail)
		if f.MaxLength > 0 {
			out = append(out, validators.MaxLengthValidator(f.MaxLength))
		}
	case KindURL:
		out = append(out, validators.ValidateURL)
		if f.MaxLength > 0 {
			out = append(out, validators.MaxLengthValidator(f.MaxLength))
		}
	case KindGenericIPAddress:
		if vs, _, err := validators.IPAddressValidators(f.Protocol, f.UnpackIPv4); err == nil {
			out = append(out, vs...)
		}
	case KindSmallInt, KindInt, KindBigInt, KindPositiveSmallInt, KindPositiveInt, KindPositiveBigInt:
		if f.Rel == nil {
			if minVal, maxVal, ok := f.IntRange(); ok {
				out = append(out, validators.MinValueValidator(minVal), validators.MaxValueValidator(maxVal))
			}
		}
	case KindDecimal:
		if f.MaxDigits > 0 || f.DecimalPlaces > 0 {
			maxD := f.MaxDigits
			if maxD == 0 {
				maxD = -1
			}
			out = append(out, validators.DecimalValidator(maxD, f.DecimalPlaces))
		}
	case KindBinary:
		if f.MaxLength > 0 {
			out = append(out, validators.MaxLengthValidator(f.MaxLength))
		}
	}
	return out
}

// AllValidators returns f's built-in validators followed by any custom
// validators registered via Meta.Validators.
func (f *Field) AllValidators() []Validator {
	defs := f.DefaultValidators()
	if len(f.Validators) == 0 {
		return defs
	}
	out := make([]Validator, 0, len(defs)+len(f.Validators))
	out = append(out, defs...)
	out = append(out, f.Validators...)
	return out
}

// Validate checks value against f's editability, choices, null, and blank
// constraints, matching Django's Field.validate(value, model_instance).
func (f *Field) Validate(value any, _ any) error {
	if !f.Editable {
		return nil
	}
	unwrapped, isNull, isEmpty := fieldValueState(f, value)
	if f.HasChoices && !isEmpty {
		matched := false
		for _, ch := range f.FlatChoices() {
			if choiceMatches(f, ch, unwrapped) {
				matched = true
				break
			}
		}
		if !matched {
			return NewValidationError(
				f.ErrorMessage("invalid_choice"),
				"invalid_choice",
				map[string]any{"value": unwrapped},
			)
		}
	}
	if isNull && !f.Null {
		return NewValidationError(f.ErrorMessage("null"), "null")
	}
	if !f.Blank && isEmpty {
		return NewValidationError(f.ErrorMessage("blank"), "blank")
	}
	return nil
}

// RunValidators runs all default and custom validators on f over a non-empty
// value, collecting every error and applying any per-code custom messages from
// Meta.ErrorMessages. It mirrors Django's Field.run_validators(value).
func (f *Field) RunValidators(value any) error {
	unwrapped, _, isEmpty := fieldValueState(f, value)
	if isEmpty {
		return nil
	}
	var errs []*ValidationError
	for _, v := range f.AllValidators() {
		err := v(unwrapped)
		if err == nil {
			continue
		}
		ve := validators.ToValidationError(err)
		if f.ErrorMessages != nil && ve.Code != "" && !ve.IsList() && !ve.IsDict() {
			if msg, ok := f.ErrorMessages[ve.Code]; ok {
				cp := *ve
				cp.Message = msg
				ve = &cp
			}
		}
		errs = append(errs, ve.ErrorList()...)
	}
	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// Clean normalizes value for f, validates null/blank/choices, and runs all
// validators on f, returning the cleaned value. It mirrors Django's
// Field.clean(value, model_instance).
func (f *Field) Clean(value any, instance any) (any, error) {
	cleaned, err := f.toCleanValue(value)
	if err != nil {
		return nil, err
	}
	if err := f.Validate(cleaned, instance); err != nil {
		return nil, err
	}
	if err := f.RunValidators(cleaned); err != nil {
		return nil, err
	}
	return cleaned, nil
}

// toCleanValue normalizes kind-specific values prior to Validate and
// RunValidators (trimming and canonicalizing IP strings, verifying UUID string
// syntax, JSON validity, and finite floats).
func (f *Field) toCleanValue(value any) (any, error) {
	unwrapped, isNull, _ := fieldValueState(f, value)
	if isNull {
		return value, nil
	}
	switch f.Kind {
	case KindGenericIPAddress:
		if s, ok := unwrapped.(string); ok {
			trimmed := strings.TrimSpace(s)
			if trimmed == "" {
				return "", nil
			}
			// Canonicalize valid IP addresses (and unpack IPv4-mapped IPv6
			// when configured); invalid strings pass through so the protocol
			// validator emits Django's exact error message.
			if norm, err := normalizeIPAddressUnpack(trimmed, f.Protocol, f.UnpackIPv4); err == nil {
				return norm, nil
			}
			return trimmed, nil
		}
	case KindUUID:
		if s, ok := unwrapped.(string); ok && s != "" {
			if _, err := ParseUUID(s); err != nil {
				msg := f.ErrorMessage("invalid")
				if msg == "" {
					msg = "“%(value)s” is not a valid UUID."
				}
				return nil, NewValidationError(msg, "invalid", map[string]any{"value": s})
			}
		}
	case KindJSON:
		if raw, ok := unwrapped.(json.RawMessage); ok && len(raw) > 0 {
			if !json.Valid(raw) {
				msg := f.ErrorMessage("invalid")
				if msg == "" {
					msg = "Value must be valid JSON."
				}
				return nil, NewValidationError(msg, "invalid", map[string]any{"value": string(raw)})
			}
		}
	case KindFloat:
		if fl, ok := asFloat64(unwrapped); ok && (math.IsNaN(fl) || math.IsInf(fl, 0)) {
			msg := f.ErrorMessage("invalid")
			if msg == "" {
				msg = "“%(value)s” value must be a float."
			}
			return nil, NewValidationError(msg, "invalid", map[string]any{"value": unwrapped})
		}
	}
	return value, nil
}

// fieldValueState inspects value for field f, unwrapping a single pointer
// layer and classifying whether the value is null and whether it is empty
// according to Django's EMPTY_VALUES semantics for Go types.
func fieldValueState(f *Field, value any) (unwrapped any, isNull, isEmpty bool) {
	if value == nil {
		return nil, true, true
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, true, true
		}
		rv = rv.Elem()
		value = rv.Interface()
	}
	if f != nil && f.Rel != nil {
		if pk, ok := extractRelationPK(rv); ok {
			if pk == 0 {
				return nil, true, true
			}
			return pk, false, false
		}
	}
	switch v := value.(type) {
	case string:
		return v, false, v == ""
	case []byte:
		return v, false, len(v) == 0
	case json.RawMessage:
		return v, false, len(v) == 0
	case Date:
		if v.IsZero() {
			return nil, true, true
		}
		return v, false, false
	case time.Time:
		if v.IsZero() {
			return nil, true, true
		}
		return v, false, false
	case netip.Addr:
		if !v.IsValid() {
			return nil, true, true
		}
		return v, false, false
	}
	return value, false, false
}

// extractRelationPK extracts the int64 primary key from a relation field value
// (such as FK[T]) via its PK() int64 method.
func extractRelationPK(rv reflect.Value) (int64, bool) {
	if !rv.IsValid() {
		return 0, false
	}
	method := rv.MethodByName("PK")
	if !method.IsValid() {
		return 0, false
	}
	out := method.Call(nil)
	if len(out) != 1 || !out[0].CanInt() {
		return 0, false
	}
	return out[0].Int(), true
}

// CleanFields validates every non-excluded field on instance, returning a
// dictionary *ValidationError if any field fails. It mirrors Django's
// Model.clean_fields(exclude=...).
func CleanFields(m *Model, instance any, opts ...CleanOption) error {
	return m.CleanFields(instance, opts...)
}

// CleanFields validates every non-excluded field on instance without hitting a
// database, returning a dictionary *ValidationError if any field fails.
func (m *Model) CleanFields(instance any, opts ...CleanOption) error {
	cfg := defaultCleanConfig(opts)
	exclude := m.normalizeExclude(cfg.exclude)
	return cleanFieldsInternal(context.Background(), nil, m, instance, exclude)
}

// ValidateUnique checks single-field unique constraints, primary-key uniqueness
// (when adding), Meta.UniqueTogether sets and the unique_for_date,
// unique_for_year and unique_for_month options against db, returning a
// dictionary *ValidationError if any constraint is violated. It mirrors
// Django's Model.validate_unique(exclude=...).
func ValidateUnique(ctx context.Context, db *DB, instance any, opts ...CleanOption) error {
	if db == nil {
		return errors.New("orm: ValidateUnique requires a non-nil *DB")
	}
	m, ok := db.Registry().ModelOf(instance)
	if !ok {
		return fmt.Errorf("orm: %T is not a registered model", instance)
	}
	cfg := defaultCleanConfig(opts)
	exclude := m.normalizeExclude(cfg.exclude)
	return validateUniqueInternal(ctx, db, m, instance, exclude, cfg)
}

// FullClean runs CleanFields, the model's optional Clean() method,
// ValidateUnique and ValidateConstraints (when enabled and db is non-nil),
// aggregating all errors into a dictionary *ValidationError. It mirrors
// Django's Model.full_clean().
func FullClean(ctx context.Context, db *DB, instance any, opts ...CleanOption) error {
	if db == nil {
		return errors.New("orm: FullClean requires a non-nil *DB (or call Model.FullClean with WithValidateUnique(false))")
	}
	m, ok := db.Registry().ModelOf(instance)
	if !ok {
		return fmt.Errorf("orm: %T is not a registered model", instance)
	}
	return m.FullClean(ctx, db, instance, opts...)
}

// FullClean runs CleanFields, the model's optional Clean() method, and, when db
// is non-nil, ValidateUnique and ValidateConstraints on instance. The database
// checks skip the fields that already failed validation.
func (m *Model) FullClean(ctx context.Context, db *DB, instance any, opts ...CleanOption) error {
	cfg := defaultCleanConfig(opts)
	exclude := m.normalizeExclude(cfg.exclude)
	errMap := map[string][]*ValidationError{}

	if err := cleanFieldsInternal(ctx, db, m, instance, exclude); err != nil {
		var ve *ValidationError
		if !errors.As(err, &ve) {
			return err
		}
		errMap = ve.UpdateErrorDict(errMap)
	}

	// Model.clean() runs even when clean_fields() collected errors, matching Django.
	if cleaner, ok := instance.(modelCleaner); ok {
		if err := cleaner.Clean(); err != nil {
			ve := validators.ToValidationError(err)
			errMap = ve.UpdateErrorDict(errMap)
		}
	}

	// Each database check also excludes the fields that failed so far, as
	// Django adds them to the same exclude set.
	dbExclude := maps.Clone(exclude)
	excludeFailed := func() {
		for name := range errMap {
			if name != NonFieldErrors {
				dbExclude[name] = true
			}
		}
	}
	if cfg.validateUnique && db != nil {
		excludeFailed()
		if err := validateUniqueInternal(ctx, db, m, instance, dbExclude, cfg); err != nil {
			var ve *ValidationError
			if !errors.As(err, &ve) {
				return err
			}
			errMap = ve.UpdateErrorDict(errMap)
		}
	}
	if cfg.validateConstraints && db != nil {
		excludeFailed()
		if err := validateConstraintsInternal(ctx, db, m, instance, dbExclude, cfg); err != nil {
			var ve *ValidationError
			if !errors.As(err, &ve) {
				return err
			}
			errMap = ve.UpdateErrorDict(errMap)
		}
	}

	if len(errMap) > 0 {
		return &ValidationError{Fields: errMap}
	}
	return nil
}

// UniqueChecks returns the normalized uniqueness checks (attribute name tuples)
// that ValidateUnique performs on m after applying exclude, matching Django's
// Model._get_unique_checks().
func (m *Model) UniqueChecks(exclude ...string) [][]string {
	excl := m.normalizeExclude(exclude)
	return m.uniqueChecks(excl, false)
}

// UniqueChecksWithConstraints returns the checks of UniqueChecks and, after
// the unique_together sets, the fields of each UniqueConstraint of
// Meta.Constraints that holds for every row: one on fields, without a
// condition. It matches Django's _get_unique_checks(include_meta_constraints=True),
// which a model formset uses to find duplicates among its forms.
func (m *Model) UniqueChecksWithConstraints(exclude ...string) [][]string {
	excl := m.normalizeExclude(exclude)
	return m.uniqueChecks(excl, true)
}

// uniqueChecks collects unique_together checks, the total unique constraints
// when withConstraints is set, and the single-field primary-key and unique
// checks, matching Django's _get_unique_checks order.
func (m *Model) uniqueChecks(exclude map[string]bool, withConstraints bool) [][]string {
	sets := m.uniqueTogether
	if withConstraints {
		sets = slices.Clone(sets)
		for _, c := range m.constraints {
			if u, ok := c.(UniqueConstraint); ok && len(u.Fields) > 0 && u.Condition.empty() {
				sets = append(sets, u.Fields)
			}
		}
	}
	var checks [][]string
	for _, set := range sets {
		attrs := make([]string, 0, len(set))
		skip := false
		for _, name := range set {
			f, ok := m.byName[name]
			if !ok {
				skip = true
				break
			}
			if exclude[f.Attr] {
				skip = true
				break
			}
			attrs = append(attrs, f.Attr)
		}
		if !skip && len(attrs) > 0 {
			checks = append(checks, attrs)
		}
	}
	for _, f := range m.fields {
		if exclude[f.Attr] {
			continue
		}
		if f.PrimaryKey || f.Unique {
			checks = append(checks, []string{f.Attr})
		}
	}
	return checks
}

// normalizeExclude resolves field references (Go struct names, attribute names,
// or column names) into a fresh set of canonical Field.Attr keys.
func (m *Model) normalizeExclude(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, raw := range names {
		if f, ok := resolveMetaField(raw, m.byName, m.byAttr, m.byColumn); ok {
			out[f.Attr] = true
		} else {
			out[raw] = true
		}
	}
	return out
}

// inspectInstance validates that instance is a struct or non-nil pointer to a
// struct matching m.GoType() and returns its struct reflect.Value.
func (m *Model) inspectInstance(instance any) (reflect.Value, error) {
	if instance == nil {
		return reflect.Value{}, fmt.Errorf("orm: expected instance of %s, got nil", m.name)
	}
	rv := reflect.ValueOf(instance)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return reflect.Value{}, fmt.Errorf("orm: expected non-nil pointer to %s", m.name)
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct || rv.Type() != m.goType {
		return reflect.Value{}, fmt.Errorf("orm: expected instance of %s, got %T", m.name, instance)
	}
	return rv, nil
}

// cleanFieldsInternal cleans every non-excluded field on instance, including
// foreign key existence checks when db is non-nil.
func cleanFieldsInternal(ctx context.Context, db *DB, m *Model, instance any, exclude map[string]bool) error {
	rv, err := m.inspectInstance(instance)
	if err != nil {
		return err
	}
	errs := map[string][]*ValidationError{}
	for _, f := range m.fields {
		if exclude[f.Attr] {
			continue
		}
		fv := rv.Field(f.Index)
		// Auto-generated primary keys have blank=True in Django and are
		// populated by the database on insert.
		if f.Kind == KindAuto && fv.IsZero() {
			continue
		}
		raw := fv.Interface()
		unwrapped, _, isEmpty := fieldValueState(f, raw)
		if f.Blank && isEmpty {
			continue
		}
		cleaned, ferr := f.Clean(raw, instance)
		if ferr != nil {
			errs[f.Attr] = validators.ToValidationError(ferr).ErrorList()
			continue
		}
		// Write back canonicalized IP strings onto addressable instances so
		// subsequent ValidateUnique checks compare canonical forms.
		if f.Kind == KindGenericIPAddress && fv.CanSet() {
			if norm, ok := cleaned.(string); ok {
				writeCleanedString(fv, norm)
			}
		}
		// Verify that a non-empty ForeignKey references an existing row.
		if f.Rel != nil && f.Rel.Target != nil && db != nil && !isEmpty {
			if fkErr := validateForeignKeyExists(ctx, db, f, unwrapped); fkErr != nil {
				var ve *ValidationError
				if !errors.As(fkErr, &ve) {
					return fkErr
				}
				errs[f.Attr] = ve.ErrorList()
			}
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// writeCleanedString stores norm into a string or *string struct field.
func writeCleanedString(fv reflect.Value, norm string) {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(norm)
	case reflect.Pointer:
		if fv.Type().Elem().Kind() == reflect.String {
			if fv.IsNil() {
				s := norm
				fv.Set(reflect.ValueOf(&s))
			} else {
				fv.Elem().SetString(norm)
			}
		}
	}
}

// validateForeignKeyExists checks that a row with primary key pk that matches
// the relation's LimitChoicesTo exists in f.Rel.Target, returning Django's
// verbatim ForeignKey invalid error if not.
func validateForeignKeyExists(ctx context.Context, db *DB, f *Field, pk any) error {
	target := f.Rel.Target
	targetPK := target.PrimaryKey()
	if targetPK == nil {
		return nil
	}
	q := newQuery(db, target).filter([]any{targetPK.Column, pk, f.Rel.LimitChoicesTo()}, false)
	exists, err := queryExists(ctx, q)
	if err != nil {
		return err
	}
	if !exists {
		return NewValidationError(
			f.ErrorMessage("invalid"),
			"invalid",
			map[string]any{
				"model": target.VerboseName(),
				"pk":    pk,
				"field": targetPK.Attr,
				"value": pk,
			},
		)
	}
	return nil
}

// validateUniqueInternal executes every applicable uniqueness check on instance
// against db.
func validateUniqueInternal(ctx context.Context, db *DB, m *Model, instance any, exclude map[string]bool, cfg cleanConfig) error {
	rv, err := m.inspectInstance(instance)
	if err != nil {
		return err
	}
	pk := m.PrimaryKey()
	adding := instanceAdding(m, rv, cfg)

	checks := m.uniqueChecks(exclude, false)
	errs := map[string][]*ValidationError{}

	for _, check := range checks {
		var filterArgs []any
		skipped := false
		for _, attr := range check {
			f, ok := m.byAttr[attr]
			if !ok {
				skipped = true
				break
			}
			fv := rv.Field(f.Index)
			if f.PrimaryKey && !adding {
				skipped = true
				break
			}
			if f.Kind == KindAuto && fv.IsZero() {
				skipped = true
				break
			}
			unwrapped, isNull, _ := fieldValueState(f, fv.Interface())
			if isNull {
				skipped = true
				break
			}
			if f.Kind == KindGenericIPAddress && f.Null {
				if s, ok := unwrapped.(string); ok && strings.TrimSpace(s) == "" {
					skipped = true
					break
				}
			}
			filterArgs = append(filterArgs, f.Column, unwrapped)
		}
		if skipped || len(filterArgs) == 0 {
			continue
		}

		q := newQuery(db, m).filter(filterArgs, false)
		if !adding && pk != nil && !rv.Field(pk.Index).IsZero() {
			q = q.filter([]any{pk.Column, rv.Field(pk.Index).Interface()}, true)
		}
		exists, err := queryExists(ctx, q)
		if err != nil {
			return err
		}
		if exists {
			key := NonFieldErrors
			if len(check) == 1 {
				key = check[0]
			}
			errs[key] = append(errs[key], uniqueErrorMessage(m, check))
		}
	}
	if err := performDateChecks(ctx, db, m, rv, exclude, adding, errs); err != nil {
		return err
	}

	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// queryExists executes SELECT EXISTS(...) for an untyped *query.
func queryExists(ctx context.Context, q *query) (bool, error) {
	sqlText, args, err := q.existsSQL()
	if errors.Is(err, errEmptyResultSet) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rows, err := q.db.conn(ctx).QueryContext(ctx, sqlText, args...)
	if err != nil {
		return false, fmt.Errorf("orm: exists %s: %w", q.model.Name(), err)
	}
	defer func() { _ = rows.Close() }()
	var exists bool
	if rows.Next() {
		if err := rows.Scan(&exists); err != nil {
			return false, fmt.Errorf("orm: exists %s: %w", q.model.Name(), err)
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("orm: exists %s: %w", q.model.Name(), err)
	}
	return exists, nil
}

// uniqueErrorMessage constructs Django's verbatim ValidationError for a single
// unique field or a multi-field unique_together constraint.
func uniqueErrorMessage(m *Model, uniqueCheck []string) *ValidationError {
	params := map[string]any{
		"model_name":   capFirst(m.VerboseName()),
		"unique_check": uniqueCheck,
	}
	if len(uniqueCheck) == 1 {
		f, _ := m.FieldByAttr(uniqueCheck[0])
		params["field_label"] = capFirst(fieldVerboseName(f))
		return NewValidationError(f.ErrorMessage("unique"), "unique", params)
	}
	labels := make([]string, len(uniqueCheck))
	for i, attr := range uniqueCheck {
		f, _ := m.FieldByAttr(attr)
		labels[i] = capFirst(fieldVerboseName(f))
	}
	params["field_labels"] = getTextList(labels, "and")
	return NewValidationError(defaultFieldErrorMessages["unique_together"], "unique_together", params)
}

// fieldVerboseName returns f.VerboseName when set, or f.Attr with underscores
// replaced by spaces, matching Django's Field.verbose_name default.
func fieldVerboseName(f *Field) string {
	if f == nil {
		return ""
	}
	if f.VerboseName != "" {
		return f.VerboseName
	}
	return strings.ReplaceAll(f.Attr, "_", " ")
}

// capFirst uppercases the first rune of s, matching Django's capfirst.
func capFirst(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+len(string(r)):]
	}
	return s
}

// getTextList joins items with commas and lastWord before the final item
// (without an Oxford comma), matching Django's django.utils.text.get_text_list.
func getTextList(items []string, lastWord string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " " + lastWord + " " + items[len(items)-1]
	}
}
