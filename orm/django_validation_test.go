package orm_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// validateAnswerToUniverse mirrors Django's tests/validation/models.py::validate_answer_to_universe.
func validateAnswerToUniverse(v any) error {
	var n int64
	switch x := v.(type) {
	case int:
		n = int64(x)
	case int64:
		n = x
	default:
		return nil
	}
	if n != 42 {
		return orm.NewValidationError(
			"This is not the answer to life, universe and everything!",
			"not42",
		)
	}
	return nil
}

// modelToValidate mirrors Django's tests/validation/models.py::ModelToValidate.
type modelToValidate struct {
	ID                   int64
	Name                 string                  `orm:"max_length=100"`
	Created              time.Time               `orm:"blank"`
	Number               *int                    `orm:"column=number_val"`
	Parent               orm.FK[modelToValidate] `orm:"null;blank;on_delete=set_null"`
	Email                string                  `orm:"type=email;blank"`
	URL                  string                  `orm:"type=url;blank"`
	FWithCustomValidator *int                    `orm:"null;blank"`
	Slug                 string                  `orm:"type=slug;blank"`
}

// Meta attaches the custom validator, verbose name and limit_choices_to
// matching Django's ModelToValidate.
func (modelToValidate) Meta() orm.Meta {
	return orm.Meta{
		VerboseName: "model to validate",
		Validators: map[string][]orm.Validator{
			"FWithCustomValidator": {validateAnswerToUniverse},
		},
		LimitChoicesTo: map[string]orm.Cond{"Parent": orm.Q("number", 10)},
	}
}

// Clean mirrors ModelToValidate.clean().
func (m *modelToValidate) Clean() error {
	if m.Number != nil && *m.Number == 11 {
		return orm.NewValidationError("Invalid number supplied!", "")
	}
	return nil
}

// uniqueFieldsModel mirrors Django's tests/validation/models.py::UniqueFieldsModel.
type uniqueFieldsModel struct {
	ID                 int64
	UniqueCharfield    string `orm:"max_length=100;unique"`
	UniqueIntegerfield int    `orm:"unique;default=42"`
	NonUniqueField     int
}

// Meta sets verbose name to match Django's UniqueFieldsModel.
func (uniqueFieldsModel) Meta() orm.Meta {
	return orm.Meta{VerboseName: "unique fields model"}
}

// uniqueTogetherModel mirrors Django's tests/validation/models.py::UniqueTogetherModel.
type uniqueTogetherModel struct {
	ID     int64
	Cfield string `orm:"max_length=100"`
	Ifield int
	Efield string `orm:"type=email"`
}

// Meta declares the two unique_together field sets.
func (uniqueTogetherModel) Meta() orm.Meta {
	return orm.Meta{
		VerboseName: "unique together model",
		UniqueTogether: [][]string{
			{"ifield", "cfield"},
			{"ifield", "efield"},
		},
	}
}

// customPKModel mirrors Django's tests/validation/models.py::CustomPKModel.
type customPKModel struct {
	MyPKField string `orm:"pk;max_length=100"`
}

// Meta sets verbose name matching CustomPKModel.
func (customPKModel) Meta() orm.Meta {
	return orm.Meta{VerboseName: "custom pk model"}
}

// customMessagesModel mirrors Django's tests/validation/models.py::CustomMessagesModel.
type customMessagesModel struct {
	ID     int64
	Other  *int `orm:"null;blank"`
	Number *int `orm:"column=number_val;null=false"`
}

// Meta configures custom error messages and validator on Number.
func (customMessagesModel) Meta() orm.Meta {
	return orm.Meta{
		Validators: map[string][]orm.Validator{
			"Number": {validateAnswerToUniverse},
		},
		ErrorMessages: map[string]map[string]string{
			"Number": {
				"null":  "NULL",
				"not42": "AAARGH",
			},
		},
	}
}

// uniqueErrorsModel mirrors Django's tests/validation/models.py::UniqueErrorsModel.
type uniqueErrorsModel struct {
	ID   int64
	Name string `orm:"max_length=100;unique"`
	No   int    `orm:"unique"`
}

// Meta sets custom unique error messages.
func (uniqueErrorsModel) Meta() orm.Meta {
	return orm.Meta{
		ErrorMessages: map[string]map[string]string{
			"Name": {"unique": "Custom unique name message."},
			"No":   {"unique": "Custom unique number message."},
		},
	}
}

// genericIPTestModel mirrors Django's tests/validation/models.py::GenericIPAddressTestModel.
type genericIPTestModel struct {
	ID        int64
	GenericIP *string `orm:"type=ip;null;blank;unique"`
	V4IP      *string `orm:"type=ip;protocol=ipv4;null;blank"`
	V6IP      *string `orm:"type=ip;protocol=ipv6;null;blank"`
}

// genericIPUnpackModel mirrors Django's tests/validation/models.py::GenericIPAddrUnpackUniqueTest.
type genericIPUnpackModel struct {
	ID              int64
	GenericV4Unpack *string `orm:"type=ip;unpack_ipv4;null;blank;unique"`
}

// valIntPtr returns a pointer to n.
func valIntPtr(n int) *int { return &n }

// valStrPtr returns a pointer to s.
func valStrPtr(s string) *string { return &s }

// assertFailsValidation runs fn and verifies it returns a *orm.ValidationError
// whose MessageDict has exactly the expected field keys.
func assertFailsValidation(t *testing.T, err error, wantFields []string) *orm.ValidationError {
	t.Helper()
	var ve *orm.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got err = %v, want *orm.ValidationError", err)
	}
	gotDict := ve.MessageDict()
	gotKeys := make([]string, 0, len(gotDict))
	for k := range gotDict {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	wantSorted := append([]string(nil), wantFields...)
	sort.Strings(wantSorted)
	djangotest.AssertEqual(t, gotKeys, wantSorted)
	return ve
}

// assertFieldFailsWithMessage runs err check for a specific field's messages.
func assertFieldFailsWithMessage(t *testing.T, err error, field string, wantMsgs []string) {
	t.Helper()
	var ve *orm.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got err = %v, want *orm.ValidationError", err)
	}
	djangotest.AssertEqual(t, ve.MessageDict()[field], wantMsgs)
}

// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_missing_required_field_raises_error
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_with_correct_value_model_validates
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_custom_validate_method
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_wrong_FK_value_raises_error
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_correct_FK_value_validates
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_limited_FK_raises_error
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_wrong_email_value_raises_error
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_correct_email_value_passes
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_wrong_url_value_raises_error
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_text_greater_that_charfields_max_length_raises_errors
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_malformed_slug_raises_error
// Ported from Django: tests/validation/tests.py::BaseModelValidationTests.test_full_clean_does_not_mutate_exclude
func TestBaseModelValidation(t *testing.T) {
	djangotest.EachDB(t, []any{
		djangotest.App("validation", &modelToValidate{}),
	}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()

		// Missing required fields (name blank, number nil).
		mtv := &modelToValidate{FWithCustomValidator: valIntPtr(42)}
		assertFailsValidation(t, orm.FullClean(ctx, db, mtv), []string{"name", "number"})

		// Valid model passes.
		mtv = &modelToValidate{Number: valIntPtr(10), Name: "Some Name"}
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, mtv), nil)

		// Custom Clean() runs even when CleanFields fails.
		mtv = &modelToValidate{Number: valIntPtr(11)}
		ve := assertFailsValidation(t, orm.FullClean(ctx, db, mtv), []string{orm.NonFieldErrors, "name"})
		djangotest.AssertEqual(t, ve.MessageDict()[orm.NonFieldErrors], []string{"Invalid number supplied!"})

		// Wrong FK value fails with Django's exact message.
		mtv = &modelToValidate{Number: valIntPtr(10), Name: "Some Name"}
		mtv.Parent.SetPK(3)
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, mtv), "parent",
			[]string{"model to validate instance with id 3 is not a valid choice."})

		// Existing FK value passes.
		parent := &modelToValidate{Number: valIntPtr(10), Name: "Some Name"}
		if err := orm.Query[modelToValidate](db).Create(ctx, parent); err != nil {
			t.Fatalf("Create parent: %v", err)
		}
		mtv = &modelToValidate{Number: valIntPtr(10), Name: "Some Name"}
		mtv.Parent.SetPK(parent.ID)
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, mtv), nil)

		// The limit_choices_to on the parent field says that a parent
		// object's number attribute must be 10, so this should fail
		// validation.
		other := &modelToValidate{Number: valIntPtr(11), Name: "Other Name"}
		if err := orm.Query[modelToValidate](db).Create(ctx, other); err != nil {
			t.Fatalf("Create other parent: %v", err)
		}
		mtv = &modelToValidate{Number: valIntPtr(10), Name: "Some Name"}
		mtv.Parent.SetPK(other.ID)
		assertFailsValidation(t, orm.FullClean(ctx, db, mtv), []string{"parent"})

		// Email validation.
		mtv = &modelToValidate{Number: valIntPtr(10), Name: "Some Name", Email: "not-an-email"}
		assertFailsValidation(t, orm.FullClean(ctx, db, mtv), []string{"email"})
		mtv = &modelToValidate{Number: valIntPtr(10), Name: "Some Name", Email: "valid@email.com"}
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, mtv), nil)

		// URL validation.
		mtv = &modelToValidate{Number: valIntPtr(10), Name: "Some Name", URL: "not a url"}
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, mtv), "url", []string{"Enter a valid URL."})

		// CharField max_length validation.
		mtv = &modelToValidate{Number: valIntPtr(10), Name: strings.Repeat("Some Name", 100)}
		assertFailsValidation(t, orm.FullClean(ctx, db, mtv), []string{"name"})

		// Slug validation.
		mtv = &modelToValidate{Number: valIntPtr(10), Name: "Some Name", Slug: "##invalid##"}
		assertFailsValidation(t, orm.FullClean(ctx, db, mtv), []string{"slug"})

		// FullClean must not mutate caller's exclude slice.
		mtv = &modelToValidate{FWithCustomValidator: valIntPtr(42)}
		exclude := []string{"number"}
		assertFailsValidation(t, orm.FullClean(ctx, db, mtv, orm.WithExclude(exclude...)), []string{"name"})
		djangotest.AssertEqual(t, len(exclude), 1)
		djangotest.AssertEqual(t, exclude[0], "number")
	})
}

// Ported from Django: tests/validation/test_validators.py::TestModelsWithValidators.test_custom_validator_passes_for_correct_value
// Ported from Django: tests/validation/test_validators.py::TestModelsWithValidators.test_custom_validator_raises_error_for_incorrect_value
func TestModelsWithValidators(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("validation", &modelToValidate{}))
	m, _ := reg.ModelOf(&modelToValidate{})
	ctx := context.Background()

	okObj := &modelToValidate{Number: valIntPtr(10), Name: "Some Name", FWithCustomValidator: valIntPtr(42)}
	djangotest.AssertEqual(t, m.FullClean(ctx, nil, okObj, orm.WithValidateUnique(false)), nil)

	badObj := &modelToValidate{Number: valIntPtr(10), Name: "Some Name", FWithCustomValidator: valIntPtr(12)}
	err := m.FullClean(ctx, nil, badObj, orm.WithValidateUnique(false))
	assertFailsValidation(t, err, []string{"f_with_custom_validator"})
	assertFieldFailsWithMessage(t, err, "f_with_custom_validator",
		[]string{"This is not the answer to life, universe and everything!"})
}

// Ported from Django: tests/validation/test_custom_messages.py::CustomMessagesTests.test_custom_simple_validator_message
// Ported from Django: tests/validation/test_custom_messages.py::CustomMessagesTests.test_custom_null_message
func TestCustomMessages(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("validation", &customMessagesModel{}))
	m, _ := reg.ModelOf(&customMessagesModel{})
	ctx := context.Background()

	cmm := &customMessagesModel{Number: valIntPtr(12)}
	assertFieldFailsWithMessage(t, m.FullClean(ctx, nil, cmm, orm.WithValidateUnique(false)), "number", []string{"AAARGH"})

	cmm = &customMessagesModel{}
	assertFieldFailsWithMessage(t, m.FullClean(ctx, nil, cmm, orm.WithValidateUnique(false)), "number", []string{"NULL"})
}

// Ported from Django: tests/validation/test_unique.py::GetUniqueCheckTests.test_unique_fields_get_collected
// Ported from Django: tests/validation/test_unique.py::GetUniqueCheckTests.test_unique_together_gets_picked_up_and_converted_to_tuple
// Ported from Django: tests/validation/test_unique.py::GetUniqueCheckTests.test_primary_key_is_considered_unique
func TestGetUniqueChecks(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("validation",
		&uniqueFieldsModel{},
		&uniqueTogetherModel{},
		&customPKModel{},
	))

	ufm, _ := reg.ModelOf(&uniqueFieldsModel{})
	djangotest.AssertEqual(t, ufm.UniqueChecks(), [][]string{
		{"id"},
		{"unique_charfield"},
		{"unique_integerfield"},
	})

	utm, _ := reg.ModelOf(&uniqueTogetherModel{})
	djangotest.AssertEqual(t, utm.UniqueChecks(), [][]string{
		{"ifield", "cfield"},
		{"ifield", "efield"},
		{"id"},
	})

	cpk, _ := reg.ModelOf(&customPKModel{})
	djangotest.AssertEqual(t, cpk.UniqueChecks(), [][]string{
		{"my_pk_field"},
	})
}

// Ported from Django: tests/validation/test_unique.py::PerformUniqueChecksTest.test_primary_key_unique_check_not_performed_when_adding_and_pk_not_specified
// Ported from Django: tests/validation/test_unique.py::PerformUniqueChecksTest.test_primary_key_unique_check_performed_when_adding_and_pk_specified
// Ported from Django: tests/validation/test_unique.py::PerformUniqueChecksTest.test_primary_key_unique_check_not_performed_when_not_adding
// Ported from Django: tests/validation/test_unique.py::PerformUniqueChecksTest.test_unique_errors
func TestPerformUniqueChecks(t *testing.T) {
	djangotest.EachDB(t, []any{
		djangotest.App("validation",
			&modelToValidate{},
			&uniqueFieldsModel{},
			&uniqueTogetherModel{},
			&uniqueErrorsModel{},
		),
	}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()

		// Adding without PK: 0 queries.
		djangotest.AssertNumQueries(t, db, 0, func() {
			mtv := &modelToValidate{Number: valIntPtr(10), Name: "Some Name"}
			djangotest.AssertEqual(t, orm.FullClean(ctx, db, mtv, orm.WithAdding(true)), nil)
		})

		// Adding with explicit PK: 1 query.
		djangotest.AssertNumQueries(t, db, 1, func() {
			mtv := &modelToValidate{ID: 123, Number: valIntPtr(10), Name: "Some Name"}
			djangotest.AssertEqual(t, orm.FullClean(ctx, db, mtv, orm.WithAdding(true)), nil)
		})

		// Not adding: 0 queries for PK check.
		djangotest.AssertNumQueries(t, db, 0, func() {
			mtv := &modelToValidate{Number: valIntPtr(10), Name: "Some Name"}
			djangotest.AssertEqual(t, orm.FullClean(ctx, db, mtv, orm.WithAdding(false)), nil)
		})

		// Single unique field violation produces Django's verbatim message.
		u1 := &uniqueFieldsModel{UniqueCharfield: "foo", UniqueIntegerfield: 42, NonUniqueField: 1}
		if err := orm.Query[uniqueFieldsModel](db).Create(ctx, u1); err != nil {
			t.Fatalf("Create u1: %v", err)
		}
		u2 := &uniqueFieldsModel{UniqueCharfield: "bar", UniqueIntegerfield: 42, NonUniqueField: 2}
		err := orm.FullClean(ctx, db, u2)
		assertFieldFailsWithMessage(t, err, "unique_integerfield",
			[]string{"Unique fields model with this Unique integerfield already exists."})

		// Editing u1 itself must NOT trigger a self-collision.
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, u1, orm.WithAdding(false)), nil)

		// unique_together violation produces Django's verbatim non-field error.
		ut1 := &uniqueTogetherModel{Cfield: "abc", Ifield: 7, Efield: "a@example.com"}
		if err := orm.Query[uniqueTogetherModel](db).Create(ctx, ut1); err != nil {
			t.Fatalf("Create ut1: %v", err)
		}
		ut2 := &uniqueTogetherModel{Cfield: "abc", Ifield: 7, Efield: "b@example.com"}
		err = orm.FullClean(ctx, db, ut2)
		assertFieldFailsWithMessage(t, err, orm.NonFieldErrors,
			[]string{"Unique together model with this Ifield and Cfield already exists."})

		// Custom unique error messages.
		ue1 := &uniqueErrorsModel{Name: "Some Name", No: 10}
		if err := orm.Query[uniqueErrorsModel](db).Create(ctx, ue1); err != nil {
			t.Fatalf("Create ue1: %v", err)
		}
		ue2 := &uniqueErrorsModel{Name: "Some Name", No: 11}
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, ue2), "name", []string{"Custom unique name message."})
		ue3 := &uniqueErrorsModel{Name: "Other Name", No: 10}
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, ue3), "no", []string{"Custom unique number message."})
	})
}

// Ported from Django: tests/validation/tests.py::GenericIPAddressFieldTests.test_correct_generic_ip_passes
// Ported from Django: tests/validation/tests.py::GenericIPAddressFieldTests.test_invalid_generic_ip_raises_error
// Ported from Django: tests/validation/tests.py::GenericIPAddressFieldTests.test_correct_v4_ip_passes
// Ported from Django: tests/validation/tests.py::GenericIPAddressFieldTests.test_invalid_v4_ip_raises_error
// Ported from Django: tests/validation/tests.py::GenericIPAddressFieldTests.test_correct_v6_ip_passes
// Ported from Django: tests/validation/tests.py::GenericIPAddressFieldTests.test_invalid_v6_ip_raises_error
// Ported from Django: tests/validation/tests.py::GenericIPAddressFieldTests.test_v6_uniqueness_detection
// Ported from Django: tests/validation/tests.py::GenericIPAddressFieldTests.test_v4_unpack_uniqueness_detection
// Ported from Django: tests/validation/tests.py::GenericIPAddressFieldTests.test_empty_generic_ip_passes
// Ported from Django: tests/validation/tests.py::GenericIPAddressFieldTests.test_multiple_invalid_ip_raises_error
func TestGenericIPAddressValidation(t *testing.T) {
	djangotest.EachDB(t, []any{
		djangotest.App("validation", &genericIPTestModel{}, &genericIPUnpackModel{}),
	}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()

		// Valid generic IPs (including surrounding whitespace).
		for _, ip := range []string{"1.2.3.4", " 1.2.3.4 ", "1.2.3.4\n", "2001::2"} {
			m := &genericIPTestModel{GenericIP: valStrPtr(ip)}
			djangotest.AssertEqual(t, orm.FullClean(ctx, db, m), nil)
		}

		// Invalid generic IPs.
		for _, ip := range []string{"294.4.2.1", "1:2"} {
			m := &genericIPTestModel{GenericIP: valStrPtr(ip)}
			assertFailsValidation(t, orm.FullClean(ctx, db, m), []string{"generic_ip"})
		}

		// IPv4 protocol.
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, &genericIPTestModel{V4IP: valStrPtr("1.2.3.4")}), nil)
		for _, ip := range []string{"294.4.2.1", "2001::2"} {
			assertFailsValidation(t, orm.FullClean(ctx, db, &genericIPTestModel{V4IP: valStrPtr(ip)}), []string{"v4_ip"})
		}

		// IPv6 protocol.
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, &genericIPTestModel{V6IP: valStrPtr("2001::2")}), nil)
		for _, ip := range []string{"1.2.3.4", "1:2"} {
			assertFailsValidation(t, orm.FullClean(ctx, db, &genericIPTestModel{V6IP: valStrPtr(ip)}), []string{"v6_ip"})
		}

		// Empty / nil IP passes when blank and null.
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, &genericIPTestModel{GenericIP: valStrPtr("")}), nil)
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, &genericIPTestModel{GenericIP: nil}), nil)

		// Multiple invalid IP fields report their protocol-specific messages.
		bad := &genericIPTestModel{
			V6IP:      valStrPtr("1.2.3.4"),
			V4IP:      valStrPtr("::ffff:10.10.10.10"),
			GenericIP: valStrPtr("fsad"),
		}
		err := orm.FullClean(ctx, db, bad)
		assertFieldFailsWithMessage(t, err, "v6_ip", []string{"Enter a valid IPv6 address."})
		assertFieldFailsWithMessage(t, err, "v4_ip", []string{"Enter a valid IPv4 address."})
		assertFieldFailsWithMessage(t, err, "generic_ip", []string{"Enter a valid IPv4 or IPv6 address."})

		// IPv6 uniqueness detection with different spellings of the same address.
		saved := &genericIPTestModel{GenericIP: valStrPtr("2001::1:0:0:0:0:2")}
		if err := orm.Query[genericIPTestModel](db).Create(ctx, saved); err != nil {
			t.Fatalf("Create saved IPv6: %v", err)
		}
		dupV6 := &genericIPTestModel{GenericIP: valStrPtr("2001:0:1::2")}
		assertFailsValidation(t, orm.FullClean(ctx, db, dupV6), []string{"generic_ip"})

		// Without unpack_ipv4, ::ffff:10.10.10.10 and 10.10.10.10 are distinct.
		mapped := &genericIPTestModel{GenericIP: valStrPtr("::ffff:10.10.10.10")}
		if err := orm.Query[genericIPTestModel](db).Create(ctx, mapped); err != nil {
			t.Fatalf("Create mapped: %v", err)
		}
		plainV4 := &genericIPTestModel{GenericIP: valStrPtr("10.10.10.10")}
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, plainV4), nil)

		// With unpack_ipv4, ::ffff:18.52.18.52 and 18.52.18.52 collide.
		unpacked := &genericIPUnpackModel{GenericV4Unpack: valStrPtr("::ffff:18.52.18.52")}
		if err := orm.Query[genericIPUnpackModel](db).Create(ctx, unpacked); err != nil {
			t.Fatalf("Create unpacked: %v", err)
		}
		dupV4 := &genericIPUnpackModel{GenericV4Unpack: valStrPtr("18.52.18.52")}
		assertFailsValidation(t, orm.FullClean(ctx, db, dupV4), []string{"generic_v4_unpack"})
	})
}

// kindValidationModel tests built-in validators for integer ranges, decimals,
// UUID strings, JSON, binary max_length, and 3-field unique_together.
type kindValidationModel struct {
	ID       int64
	PosSmall int             `orm:"type=positive_smallint"`
	Price    orm.Decimal     `orm:"max_digits=5;decimal_places=2"`
	Code     string          `orm:"type=uuid;blank"`
	Payload  json.RawMessage `orm:"blank"`
	Blob     []byte          `orm:"max_length=4;editable=true;blank"`
	A        *string         `orm:"max_length=10;null;blank"`
	B        string          `orm:"max_length=10;blank"`
	C        string          `orm:"max_length=10;blank"`
}

// Meta declares a 3-field unique_together to exercise get_text_list formatting.
func (kindValidationModel) Meta() orm.Meta {
	return orm.Meta{
		VerboseName:    "item",
		UniqueTogether: [][]string{{"A", "B", "C"}},
	}
}

// TestKindDefaultValidatorsAndUniqueTogether verifies built-in kind validators
// (integer range, DecimalValidator, UUID syntax, JSON validity, binary
// max_length) and 3-field unique_together (including NULL non-collision) on
// both dialects.
func TestKindDefaultValidatorsAndUniqueTogether(t *testing.T) {
	djangotest.EachDB(t, []any{
		djangotest.App("validation", &kindValidationModel{}),
	}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()

		// Valid instance passes.
		valid := &kindValidationModel{
			PosSmall: 100,
			Price:    orm.MustDecimal("123.45"),
			Code:     "550e8400-e29b-41d4-a716-446655440000",
			Payload:  json.RawMessage(`{"ok":true}`),
			Blob:     []byte{1, 2, 3, 4},
			A:        valStrPtr("x"),
			B:        "y",
			C:        "z",
		}
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, valid), nil)

		// Negative positive_smallint fails MinValueValidator(0).
		neg := *valid
		neg.PosSmall = -1
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, &neg), "pos_small",
			[]string{"Ensure this value is greater than or equal to 0."})

		// Overflow positive_smallint fails MaxValueValidator(32767).
		over := *valid
		over.PosSmall = 40000
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, &over), "pos_small",
			[]string{"Ensure this value is less than or equal to 32767."})

		// DecimalValidator checks total digits, decimal places, and whole digits.
		badDec := *valid
		badDec.Price = orm.MustDecimal("1234.56")
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, &badDec), "price",
			[]string{"Ensure that there are no more than 5 digits in total."})

		badPlaces := *valid
		badPlaces.Price = orm.MustDecimal("1.234")
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, &badPlaces), "price",
			[]string{"Ensure that there are no more than 2 decimal places."})

		badWhole := *valid
		badWhole.Price = orm.MustDecimal("1234")
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, &badWhole), "price",
			[]string{"Ensure that there are no more than 3 digits before the decimal point."})

		// Invalid UUID string.
		badUUID := *valid
		badUUID.Code = "not-a-uuid"
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, &badUUID), "code",
			[]string{"“not-a-uuid” is not a valid UUID."})

		// Invalid JSON raw message.
		badJSON := *valid
		badJSON.Payload = json.RawMessage(`{invalid`)
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, &badJSON), "payload",
			[]string{"Value must be valid JSON."})

		// Binary max_length.
		badBlob := *valid
		badBlob.Blob = []byte{1, 2, 3, 4, 5}
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, &badBlob), "blob",
			[]string{"Ensure this value has at most 4 characters (it has 5)."})

		// 3-field unique_together formats without Oxford comma: "A, B and C".
		if err := orm.Query[kindValidationModel](db).Create(ctx, valid); err != nil {
			t.Fatalf("Create valid: %v", err)
		}
		dup := *valid
		dup.ID = 0
		assertFieldFailsWithMessage(t, orm.FullClean(ctx, db, &dup), orm.NonFieldErrors,
			[]string{"Item with this A, B and C already exists."})

		// NULL in any unique_together column skips collision check.
		withNull := *valid
		withNull.ID = 0
		withNull.A = nil
		if err := orm.Query[kindValidationModel](db).Create(ctx, &withNull); err != nil {
			t.Fatalf("Create withNull: %v", err)
		}
		withNull2 := *valid
		withNull2.ID = 0
		withNull2.A = nil
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, &withNull2), nil)
	})
}
