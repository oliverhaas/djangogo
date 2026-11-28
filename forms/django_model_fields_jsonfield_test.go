package forms_test

import (
	"encoding/json"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// JSONModel is Django's model_fields JSONModel: a JSON field that is not
// null.
type JSONModel struct {
	ID    int64
	Value json.RawMessage
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestFormField.test_formfield
func TestJSONFieldFormFieldFormfield(t *testing.T) {
	models := []any{djangotest.App("model_fields", &JSONModel{})}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		form := forms.NewModelForm[JSONModel](db, forms.WithFields("value"))
		formField, _ := form.FieldDef("value")
		if _, ok := formField.(*forms.JSONField); !ok {
			t.Errorf("value is %T, want a *forms.JSONField", formField)
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestFormField.test_formfield_custom_encoder_decoder
func TestJSONFieldFormFieldFormfieldCustomEncoderDecoder(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a custom JSON encoder and decoder on a model field")
}
