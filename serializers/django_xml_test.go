package serializers_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// Ported from Django: tests/serializers/test_xml.py::XmlSerializerTestCase.test_control_char_failure
// Check that serializing XML 1.0 disallowed control characters fails while HT, LF, and CR succeed.
func TestXmlSerializerControlCharFailure(t *testing.T) {
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		data := seedBaseTestData(t, db)
		data.a1.Headline = "This contains \u0001 control \u0011 chars"
		wantMsg := fmt.Sprintf("Article.headline (pk:%d) contains unserializable characters", data.a1.ID)
		_, err := serializers.Serialize(ctx, db, "xml", []*Article{data.a1})
		if err == nil || !strings.Contains(err.Error(), wantMsg) {
			t.Errorf("Serialize error = %v, want substring %q", err, wantMsg)
		}

		data.a1.Headline = "HT \u0009, LF \u000a, and CR \u000d are allowed"
		serialized, err := serializers.Serialize(ctx, db, "xml", []*Article{data.a1})
		if err != nil {
			t.Fatalf("Serialize allowed control chars: %v", err)
		}
		djangotest.AssertEqual(t, strings.Contains(serialized, "HT \t, LF \n, and CR \r are allowed"), true)
	})
}

// Ported from Django: tests/serializers/test_xml.py::XmlSerializerTestCase.test_no_dtd
// Check that the XML deserializer rejects a DOCTYPE declaration with DTDForbiddenError.
func TestXmlSerializerNoDTD(t *testing.T) {
	const xmlInput = `<?xml version="1.0" standalone="no"?>` +
		`<!DOCTYPE example SYSTEM "http://example.com/example.dtd">`
	db := newTestDeserializerDB(t)
	ctx := context.Background()
	next, stop := iter.Pull2(serializers.Deserialize(ctx, db, "xml", strings.NewReader(xmlInput)))
	defer stop()

	_, err, ok := next()
	djangotest.AssertEqual(t, ok, true)
	var dtdErr *serializers.DTDForbiddenError
	if !errors.As(err, &dtdErr) {
		t.Fatalf("error = %v (%T), want *serializers.DTDForbiddenError", err, err)
	}
	djangotest.AssertEqual(
		t,
		dtdErr.Error(),
		"DTDForbidden(name='example', system_id='http://example.com/example.dtd', public_id=None)",
	)
}
