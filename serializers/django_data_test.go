package serializers_test

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
	"github.com/oliverhaas/djangogo/validators"
)

// dataTestHelper pairs an object creation function with an object comparison
// function, like Django's TestHelper namedtuple in test_data.py.
type dataTestHelper struct {
	name          string
	createObject  func(ctx context.Context, t *testing.T, db *orm.DB, pk int64, model any, data any) []any
	compareObject func(ctx context.Context, t *testing.T, db *orm.DB, pk int64, model any, data any)
}

// dataTestRow is one (helper, pk, model, data) entry in serializerTestData,
// corresponding to a tuple in Django's test_data list.
type dataTestRow struct {
	helper dataTestHelper
	pk     int64
	model  any
	data   any
}

// unsupportedDataTestRow records a Django test_data row that needs an
// unimplemented Go or ORM feature.
type unsupportedDataTestRow struct {
	feature   string
	helper    string
	pk        any
	modelName string
	data      any
}

// imRowData holds the left, right, and optional extra fields for an
// Intermediate row in test_data.
type imRowData struct {
	right int64
	left  int64
	extra string
	hasEx bool
}

// dataObj creates and compares a model with an ID primary key and a Data field,
// like Django's data_obj helper.
var dataObj = dataTestHelper{
	name:          "data_obj",
	createObject:  dataCreate,
	compareObject: dataCompare,
}

// genericObj creates and compares a GenericData instance with attached Tag
// rows, like Django's generic_obj helper.
var genericObj = dataTestHelper{
	name:          "generic_obj",
	createObject:  genericCreate,
	compareObject: genericCompare,
}

// fkObj creates and compares a model with an ID primary key and a Data foreign
// key field, like Django's fk_obj helper.
var fkObj = dataTestHelper{
	name:          "fk_obj",
	createObject:  fkCreate,
	compareObject: fkCompare,
}

// m2mObj creates and compares a model with an ID primary key and a Data M2M
// relation, like Django's m2m_obj helper.
var m2mObj = dataTestHelper{
	name:          "m2m_obj",
	createObject:  m2mCreate,
	compareObject: m2mCompare,
}

// im2mObj creates and compares an M2MIntermediateData instance, like Django's
// im2m_obj helper.
var im2mObj = dataTestHelper{
	name:          "im2m_obj",
	createObject:  im2mCreate,
	compareObject: im2mCompare,
}

// imObj creates and compares an Intermediate through-model row, like Django's
// im_obj helper.
var imObj = dataTestHelper{
	name:          "im_obj",
	createObject:  imCreate,
	compareObject: imCompare,
}

// o2oObj creates and compares an O2OData instance whose primary key is its
// Data OneToOneField, like Django's o2o_obj helper.
var o2oObj = dataTestHelper{
	name:          "o2o_obj",
	createObject:  o2oCreate,
	compareObject: o2oCompare,
}

// pkObj creates and compares a model whose Data field is its primary key, like
// Django's pk_obj helper.
var pkObj = dataTestHelper{
	name:          "pk_obj",
	createObject:  pkCreate,
	compareObject: pkCompare,
}

// inheritedObj creates and compares an InheritAbstractModel instance, like
// Django's inherited_obj helper.
var inheritedObj = dataTestHelper{
	name:          "inherited_obj",
	createObject:  inheritedCreate,
	compareObject: inheritedCompare,
}

// dataUUIDObj is the shared random UUID value used across UUID primary key
// rows, like Django's uuid_obj = uuid.uuid4().
var dataUUIDObj = orm.NewUUID()

// newModelStruct allocates a new pointer to the model struct type of proto and
// returns the *orm.Model descriptor, the addressable struct reflect.Value, and
// the pointer interface.
func newModelStruct(t *testing.T, db *orm.DB, proto any) (*orm.Model, reflect.Value, any) {
	t.Helper()
	m, ok := db.Registry().ModelOf(proto)
	if !ok {
		t.Fatalf("model %T is not registered", proto)
	}
	ptr := reflect.New(m.GoType())
	return m, ptr.Elem(), ptr.Interface()
}

// setStructDataField assigns data onto struct field fv, for both pointer and
// value fields as well as nil.
func setStructDataField(t *testing.T, fv reflect.Value, data any) {
	t.Helper()
	if data == nil {
		fv.SetZero()
		return
	}
	rv := reflect.ValueOf(data)
	if fv.Kind() == reflect.Pointer {
		ptr := reflect.New(fv.Type().Elem())
		setStructDataField(t, ptr.Elem(), data)
		fv.Set(ptr)
		return
	}
	if rv.Type().AssignableTo(fv.Type()) {
		fv.Set(rv)
		return
	}
	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fv.SetInt(rv.Int())
		return
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		fv.SetUint(uint64(rv.Int()))
		return
	case reflect.Float32, reflect.Float64:
		fv.SetFloat(rv.Float())
		return
	}
	t.Fatalf("cannot assign %T to %s", data, fv.Type())
}

// getModelByID queries the single row of proto's model matching id=pk.
func getModelByID(ctx context.Context, t *testing.T, db *orm.DB, proto any, pk int64) (*orm.Model, reflect.Value, any) {
	t.Helper()
	m, ok := db.Registry().ModelOf(proto)
	if !ok {
		t.Fatalf("model %T is not registered", proto)
	}
	rows, err := orm.QueryModel(db, m).Filter("id", pk).All(ctx)
	if err != nil {
		t.Fatalf("get %s(id=%d): %v", m.Name(), pk, err)
	}
	if len(rows) != 1 {
		t.Fatalf("get %s(id=%d): got %d rows, want 1", m.Name(), pk, len(rows))
	}
	return m, reflect.ValueOf(rows[0]).Elem(), rows[0]
}

// getModelByData queries the single row of proto's model matching data=data.
func getModelByData(ctx context.Context, t *testing.T, db *orm.DB, proto any, data any) (*orm.Model, reflect.Value, any) {
	t.Helper()
	m, ok := db.Registry().ModelOf(proto)
	if !ok {
		t.Fatalf("model %T is not registered", proto)
	}
	rows, err := orm.QueryModel(db, m).Filter("data", data).All(ctx)
	if err != nil {
		t.Fatalf("get %s(data=%v): %v", m.Name(), data, err)
	}
	if len(rows) != 1 {
		t.Fatalf("get %s(data=%v): got %d rows, want 1", m.Name(), data, len(rows))
	}
	return m, reflect.ValueOf(rows[0]).Elem(), rows[0]
}

// dataCreate creates an instance with ID=pk and Data=data via orm.SaveRaw.
func dataCreate(ctx context.Context, t *testing.T, db *orm.DB, pk int64, proto any, data any) []any {
	t.Helper()
	_, sv, ptr := newModelStruct(t, db, proto)
	sv.FieldByName("ID").SetInt(pk)
	setStructDataField(t, sv.FieldByName("Data"), data)
	if err := orm.SaveRaw(ctx, db, ptr); err != nil {
		t.Fatalf("SaveRaw(%T pk=%d): %v", proto, pk, err)
	}
	return []any{ptr}
}

// dataCompare fetches the row of proto with id=pk and asserts that its Data
// field matches data.
func dataCompare(ctx context.Context, t *testing.T, db *orm.DB, pk int64, proto any, data any) {
	t.Helper()
	_, sv, _ := getModelByID(ctx, t, db, proto, pk)
	fv := sv.FieldByName("Data")

	if _, isBin := proto.(*BinaryData); isBin {
		got, _ := fv.Interface().([]byte)
		if data == nil {
			if got != nil {
				t.Errorf("Objects with PK=%d not equal; expected nil, got %v", pk, got)
			}
			return
		}
		want := data.([]byte)
		if !bytes.Equal(want, got) {
			t.Errorf("Objects with PK=%d not equal; expected %x, got %x", pk, want, got)
		}
		return
	}

	if fv.Kind() == reflect.Pointer {
		if data == nil {
			if !fv.IsNil() {
				t.Errorf("Objects with PK=%d not equal; expected nil, got %v", pk, fv.Elem().Interface())
			}
			return
		}
		if fv.IsNil() {
			t.Errorf("Objects with PK=%d not equal; expected %v (%T), got nil", pk, data, data)
			return
		}
		fv = fv.Elem()
	}

	assertDataValueEqual(t, pk, data, fv.Interface())
}

// readAnyInt extracts a signed or unsigned integer from v as int64.
func readAnyInt(v any) int64 {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint())
	default:
		return rv.Int()
	}
}

// assertDataValueEqual compares expected and actual scalar field values across
// Decimal, time.Time, integer, float, boolean, string, Date, Time, and UUID.
func assertDataValueEqual(t *testing.T, pk int64, want, got any) {
	t.Helper()
	switch w := want.(type) {
	case orm.Decimal:
		g, ok := got.(orm.Decimal)
		if !ok || !w.Equal(g) {
			t.Errorf("Objects with PK=%d not equal; expected '%v' (%T), got '%v' (%T)", pk, want, want, got, got)
		}
	case time.Time:
		g, ok := got.(time.Time)
		if !ok || !w.Equal(g) {
			t.Errorf("Objects with PK=%d not equal; expected '%v' (%T), got '%v' (%T)", pk, want, want, got, got)
		}
	case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
		if readAnyInt(got) != readAnyInt(w) {
			t.Errorf("Objects with PK=%d not equal; expected '%v' (%T), got '%v' (%T)", pk, want, want, got, got)
		}
	default:
		if !reflect.DeepEqual(want, got) {
			t.Errorf("Objects with PK=%d not equal; expected '%v' (%T), got '%v' (%T)", pk, want, want, got, got)
		}
	}
}

// genericCreate creates a GenericData row via orm.SaveRaw and creates its Tag
// rows via orm.GenericSet.
func genericCreate(ctx context.Context, t *testing.T, db *orm.DB, pk int64, _ any, data any) []any {
	t.Helper()
	items := data.([]string)
	instance := &GenericData{ID: pk, Data: items[0]}
	if err := orm.SaveRaw(ctx, db, instance); err != nil {
		t.Fatalf("SaveRaw(GenericData pk=%d): %v", pk, err)
	}
	mgr := orm.GenericSet[Tag](db, instance, "tags")
	for _, tag := range items[1:] {
		if err := mgr.Create(ctx, &Tag{Data: tag}); err != nil {
			t.Fatalf("GenericSet.Create(tag=%q): %v", tag, err)
		}
	}
	return []any{instance}
}

// genericCompare fetches the GenericData row with id=pk and compares its Data
// and its tags ordered by id.
func genericCompare(ctx context.Context, t *testing.T, db *orm.DB, pk int64, _ any, data any) {
	t.Helper()
	items := data.([]string)
	instance, err := orm.Query[GenericData](db).Get(ctx, "id", pk)
	if err != nil {
		t.Fatalf("Get(GenericData id=%d): %v", pk, err)
	}
	djangotest.AssertEqual(t, instance.Data, items[0])
	tags, err := orm.GenericSet[Tag](db, &instance, "tags").QuerySet().OrderBy("id").All(ctx)
	if err != nil {
		t.Fatalf("GenericSet(tags).OrderBy(id): %v", err)
	}
	gotTags := make([]string, len(tags))
	for i, tag := range tags {
		gotTags[i] = tag.Data
	}
	djangotest.AssertEqual(t, gotTags, items[1:])
}

// fkCreate creates an instance with ID=pk and its Data FK set to data via
// orm.SaveRaw.
func fkCreate(ctx context.Context, t *testing.T, db *orm.DB, pk int64, proto any, data any) []any {
	t.Helper()
	_, sv, ptr := newModelStruct(t, db, proto)
	sv.FieldByName("ID").SetInt(pk)
	if data != nil {
		targetPK := reflect.ValueOf(data).Int()
		sv.FieldByName("Data").Addr().MethodByName("SetPK").Call([]reflect.Value{reflect.ValueOf(targetPK)})
	}
	if err := orm.SaveRaw(ctx, db, ptr); err != nil {
		t.Fatalf("SaveRaw(%T pk=%d): %v", proto, pk, err)
	}
	return []any{ptr}
}

// fkCompare fetches the row of proto with id=pk and asserts that its Data FK
// matches data.
func fkCompare(ctx context.Context, t *testing.T, db *orm.DB, pk int64, proto any, data any) {
	t.Helper()
	_, sv, _ := getModelByID(ctx, t, db, proto, pk)
	gotPK := sv.FieldByName("Data").Interface().(interface{ PK() int64 }).PK()
	var wantPK int64
	if data != nil {
		wantPK = reflect.ValueOf(data).Int()
	}
	djangotest.AssertEqual(t, gotPK, wantPK)
}

// m2mCreate creates an instance with ID=pk via orm.SaveRaw and sets its Data
// M2M primary keys via orm.SetManyPKs.
func m2mCreate(ctx context.Context, t *testing.T, db *orm.DB, pk int64, proto any, data any) []any {
	t.Helper()
	m, sv, ptr := newModelStruct(t, db, proto)
	sv.FieldByName("ID").SetInt(pk)
	if err := orm.SaveRaw(ctx, db, ptr); err != nil {
		t.Fatalf("SaveRaw(%T pk=%d): %v", proto, pk, err)
	}
	m2mField, ok := m.ManyToManyField("data")
	if !ok {
		t.Fatalf("model %s has no M2M field data", m.Name())
	}
	ids := data.([]int64)
	pks := make([]any, len(ids))
	for i, id := range ids {
		pks[i] = id
	}
	if err := orm.SetManyPKs(ctx, db, ptr, m2mField, pks); err != nil {
		t.Fatalf("SetManyPKs(%s pk=%d): %v", m.Name(), pk, err)
	}
	return []any{ptr}
}

// m2mCompare fetches the row of proto with id=pk and compares its Data M2M
// target IDs ordered by id.
func m2mCompare(ctx context.Context, t *testing.T, db *orm.DB, pk int64, proto any, data any) {
	t.Helper()
	m, _, ptr := getModelByID(ctx, t, db, proto, pk)
	m2mField, _ := m.ManyToManyField("data")
	qs, err := orm.RelatedMany(db, ptr, m2mField)
	if err != nil {
		t.Fatalf("RelatedMany(%s id=%d): %v", m.Name(), pk, err)
	}
	rows, err := qs.OrderBy("id").All(ctx)
	if err != nil {
		t.Fatalf("RelatedMany.OrderBy(id): %v", err)
	}
	got := make([]int64, len(rows))
	for i, row := range rows {
		got[i] = reflect.ValueOf(row).Elem().FieldByName("ID").Int()
	}
	want := data.([]int64)
	djangotest.AssertEqual(t, got, want)
}

// im2mCreate creates an M2MIntermediateData instance with ID=pk via orm.SaveRaw.
func im2mCreate(ctx context.Context, t *testing.T, db *orm.DB, pk int64, _ any, _ any) []any {
	t.Helper()
	instance := &M2MIntermediateData{ID: pk}
	if err := orm.SaveRaw(ctx, db, instance); err != nil {
		t.Fatalf("SaveRaw(M2MIntermediateData pk=%d): %v", pk, err)
	}
	return []any{instance}
}

// im2mCompare verifies that the M2MIntermediateData row with id=pk exists.
func im2mCompare(ctx context.Context, t *testing.T, db *orm.DB, pk int64, _ any, _ any) {
	t.Helper()
	if _, err := orm.Query[M2MIntermediateData](db).Get(ctx, "id", pk); err != nil {
		t.Fatalf("Get(M2MIntermediateData id=%d): %v", pk, err)
	}
}

// imCreate creates an Intermediate row with ID=pk, Left, Right, and Extra via
// orm.SaveRaw, applying the model's default when extra is not provided.
func imCreate(ctx context.Context, t *testing.T, db *orm.DB, pk int64, _ any, data any) []any {
	t.Helper()
	d := data.(imRowData)
	instance := &Intermediate{
		ID:    pk,
		Right: fkTo[Anchor](d.right),
		Left:  fkTo[M2MIntermediateData](d.left),
		Extra: "doesn't matter",
	}
	if d.hasEx {
		instance.Extra = d.extra
	}
	if err := orm.SaveRaw(ctx, db, instance); err != nil {
		t.Fatalf("SaveRaw(Intermediate pk=%d): %v", pk, err)
	}
	return []any{instance}
}

// imCompare fetches the Intermediate row with id=pk and compares its Left,
// Right, and Extra fields.
func imCompare(ctx context.Context, t *testing.T, db *orm.DB, pk int64, _ any, data any) {
	t.Helper()
	d := data.(imRowData)
	instance, err := orm.Query[Intermediate](db).Get(ctx, "id", pk)
	if err != nil {
		t.Fatalf("Get(Intermediate id=%d): %v", pk, err)
	}
	djangotest.AssertEqual(t, instance.Left.PK(), d.left)
	djangotest.AssertEqual(t, instance.Right.PK(), d.right)
	wantExtra := "doesn't matter"
	if d.hasEx {
		wantExtra = d.extra
	}
	djangotest.AssertEqual(t, instance.Extra, wantExtra)
}

// o2oCreate creates an O2OData row with Data=data via orm.SaveRaw.
func o2oCreate(ctx context.Context, t *testing.T, db *orm.DB, _ int64, _ any, data any) []any {
	t.Helper()
	targetPK := reflect.ValueOf(data).Int()
	instance := &O2OData{Data: o2oTo[Anchor](targetPK)}
	if err := orm.SaveRaw(ctx, db, instance); err != nil {
		t.Fatalf("SaveRaw(O2OData data=%d): %v", targetPK, err)
	}
	return []any{instance}
}

// o2oCompare fetches the O2OData row with data=data and compares its Data PK.
func o2oCompare(ctx context.Context, t *testing.T, db *orm.DB, _ int64, _ any, data any) {
	t.Helper()
	targetPK := reflect.ValueOf(data).Int()
	instance, err := orm.Query[O2OData](db).Get(ctx, "data", targetPK)
	if err != nil {
		t.Fatalf("Get(O2OData data=%d): %v", targetPK, err)
	}
	djangotest.AssertEqual(t, instance.Data.PK(), targetPK)
}

// pkCreate creates an instance whose Data field is its primary key via
// orm.SaveRaw.
func pkCreate(ctx context.Context, t *testing.T, db *orm.DB, _ int64, proto any, data any) []any {
	t.Helper()
	_, sv, ptr := newModelStruct(t, db, proto)
	setStructDataField(t, sv.FieldByName("Data"), data)
	if err := orm.SaveRaw(ctx, db, ptr); err != nil {
		t.Fatalf("SaveRaw(%T data=%v): %v", proto, data, err)
	}
	return []any{ptr}
}

// pkCompare fetches the row of proto with data=data and compares its Data
// primary key value.
func pkCompare(ctx context.Context, t *testing.T, db *orm.DB, pk int64, proto any, data any) {
	t.Helper()
	_, sv, _ := getModelByData(ctx, t, db, proto, data)
	got := sv.FieldByName("Data").Interface()
	assertDataValueEqual(t, pk, data, got)
}

// inheritedData holds parent_data and child_data for InheritAbstractModel.
type inheritedData struct {
	childData  int64
	parentData int64
}

// inheritedCreate creates an InheritAbstractModel row with ID=pk.
func inheritedCreate(ctx context.Context, t *testing.T, db *orm.DB, pk int64, _ any, data any) []any {
	t.Helper()
	d := data.(inheritedData)
	instance := &InheritAbstractModel{
		ID:         pk,
		ChildData:  d.childData,
		ParentData: d.parentData,
	}
	if err := orm.Query[InheritAbstractModel](db).Save(ctx, instance); err != nil {
		t.Fatalf("Save(InheritAbstractModel pk=%d): %v", pk, err)
	}
	return []any{instance}
}

// inheritedCompare fetches the InheritAbstractModel row with id=pk and compares
// its ChildData and ParentData fields.
func inheritedCompare(ctx context.Context, t *testing.T, db *orm.DB, pk int64, _ any, data any) {
	t.Helper()
	d := data.(inheritedData)
	instance, err := orm.Query[InheritAbstractModel](db).Get(ctx, "id", pk)
	if err != nil {
		t.Fatalf("Get(InheritAbstractModel id=%d): %v", pk, err)
	}
	djangotest.AssertEqual(t, instance.ChildData, d.childData)
	djangotest.AssertEqual(t, instance.ParentData, d.parentData)
}

// serializerTestData is the ported table of rows from Django's
// tests/serializers/test_data.py::test_data, in the same order and with the
// same primary keys and data values.
var serializerTestData = []dataTestRow{
	// Format: (test helper, PK value, Model Prototype, data)
	{dataObj, 1, &BinaryData{}, []byte{0x05, 0xfd, 0x00}},
	{dataObj, 5, &BooleanData{}, true},
	{dataObj, 6, &BooleanData{}, false},
	{dataObj, 7, &BooleanData{}, nil},
	{dataObj, 10, &CharData{}, "Test Char Data"},
	{dataObj, 11, &CharData{}, ""},
	{dataObj, 12, &CharData{}, "None"},
	{dataObj, 13, &CharData{}, "null"},
	{dataObj, 14, &CharData{}, "NULL"},
	{dataObj, 16, &CharData{}, "\u00a5"},
	{dataObj, 20, &DateData{}, orm.NewDate(2006, time.June, 16)},
	{dataObj, 21, &DateData{}, nil},
	{dataObj, 30, &DateTimeData{}, time.Date(2006, time.June, 16, 10, 42, 37, 0, time.UTC)},
	{dataObj, 31, &DateTimeData{}, nil},
	{dataObj, 40, &EmailData{}, "hovercraft@example.com"},
	{dataObj, 42, &EmailData{}, ""},
	// Rows 50, 52 (FileData) and 60, 62 (FilePathData) are in unsupportedSerializerTestData.
	{dataObj, 70, &DecimalData{}, orm.MustDecimal("12.345")},
	{dataObj, 71, &DecimalData{}, orm.MustDecimal("-12.345")},
	{dataObj, 72, &DecimalData{}, orm.MustDecimal("0.0")},
	{dataObj, 73, &DecimalData{}, nil},
	{dataObj, 74, &FloatData{}, 12.345},
	{dataObj, 75, &FloatData{}, -12.345},
	{dataObj, 76, &FloatData{}, 0.0},
	{dataObj, 77, &FloatData{}, nil},
	{dataObj, 80, &IntegerData{}, int64(123456789)},
	{dataObj, 81, &IntegerData{}, int64(-123456789)},
	{dataObj, 82, &IntegerData{}, int64(0)},
	{dataObj, 83, &IntegerData{}, nil},
	{dataObj, 95, &GenericIPAddressData{}, "fe80:1424:2223:6cff:fe8a:2e8a:2151:abcd"},
	{dataObj, 96, &GenericIPAddressData{}, nil},
	{dataObj, 110, &PositiveBigIntegerData{}, int64(9223372036854775807)},
	{dataObj, 111, &PositiveBigIntegerData{}, nil},
	{dataObj, 120, &PositiveIntegerData{}, int64(123456789)},
	{dataObj, 121, &PositiveIntegerData{}, nil},
	{dataObj, 130, &PositiveSmallIntegerData{}, int16(12)},
	{dataObj, 131, &PositiveSmallIntegerData{}, nil},
	{dataObj, 140, &SlugData{}, "this-is-a-slug"},
	{dataObj, 142, &SlugData{}, ""},
	{dataObj, 150, &SmallData{}, int16(12)},
	{dataObj, 151, &SmallData{}, int16(-12)},
	{dataObj, 152, &SmallData{}, int16(0)},
	{dataObj, 153, &SmallData{}, nil},
	{dataObj, 160, &TextData{}, "This is a long piece of text.\nIt contains line breaks.\nSeveral of them.\nThe end."},
	{dataObj, 161, &TextData{}, ""},
	{dataObj, 170, &TimeData{}, orm.NewTime(10, 42, 37, 0)},
	{dataObj, 171, &TimeData{}, nil},
	{genericObj, 200, &GenericData{}, []string{"Generic Object 1", "tag1", "tag2"}},
	{genericObj, 201, &GenericData{}, []string{"Generic Object 2", "tag2", "tag3"}},
	{dataObj, 300, &Anchor{}, "Anchor 1"},
	{dataObj, 301, &Anchor{}, "Anchor 2"},
	{dataObj, 302, &UniqueAnchor{}, "UAnchor 1"},
	{fkObj, 400, &FKData{}, int64(300)},
	{fkObj, 401, &FKData{}, int64(500)},
	{fkObj, 402, &FKData{}, nil},
	{m2mObj, 410, &M2MData{}, []int64{}},
	{m2mObj, 411, &M2MData{}, []int64{300, 301}},
	{m2mObj, 412, &M2MData{}, []int64{500, 501}},
	{m2mObj, 413, &M2MData{}, []int64{300, 301, 500, 501}},
	{o2oObj, 0, &O2OData{}, int64(300)},
	{o2oObj, 0, &O2OData{}, int64(500)},
	{fkObj, 430, &FKSelfData{}, int64(431)},
	{fkObj, 431, &FKSelfData{}, int64(430)},
	{fkObj, 432, &FKSelfData{}, nil},
	{m2mObj, 440, &M2MSelfData{}, []int64{}},
	{m2mObj, 441, &M2MSelfData{}, []int64{}},
	{m2mObj, 442, &M2MSelfData{}, []int64{440, 441}},
	{m2mObj, 443, &M2MSelfData{}, []int64{445, 446}},
	{m2mObj, 444, &M2MSelfData{}, []int64{440, 441, 445, 446}},
	{m2mObj, 445, &M2MSelfData{}, []int64{}},
	{m2mObj, 446, &M2MSelfData{}, []int64{}},
	// Rows 450, 451, 452 (FKDataToField) are in unsupportedSerializerTestData.
	{fkObj, 460, &FKDataToO2O{}, int64(300)},
	{im2mObj, 470, &M2MIntermediateData{}, nil},
	{imObj, 480, &Intermediate{}, imRowData{right: 300, left: 470}},
	{imObj, 481, &Intermediate{}, imRowData{right: 300, left: 490}},
	{imObj, 482, &Intermediate{}, imRowData{right: 500, left: 470}},
	{imObj, 483, &Intermediate{}, imRowData{right: 500, left: 490}},
	{imObj, 484, &Intermediate{}, imRowData{right: 300, left: 470, extra: "extra", hasEx: true}},
	{imObj, 485, &Intermediate{}, imRowData{right: 300, left: 490, extra: "extra", hasEx: true}},
	{imObj, 486, &Intermediate{}, imRowData{right: 500, left: 470, extra: "extra", hasEx: true}},
	{imObj, 487, &Intermediate{}, imRowData{right: 500, left: 490, extra: "extra", hasEx: true}},
	{im2mObj, 490, &M2MIntermediateData{}, []int64{}},
	{dataObj, 500, &Anchor{}, "Anchor 3"},
	{dataObj, 501, &Anchor{}, "Anchor 4"},
	{dataObj, 502, &UniqueAnchor{}, "UAnchor 2"},
	{pkObj, 601, &BooleanPKData{}, true},
	{pkObj, 602, &BooleanPKData{}, false},
	{pkObj, 610, &CharPKData{}, "Test Char PKData"},
	{pkObj, 620, &DatePKData{}, orm.NewDate(2006, time.June, 16)},
	{pkObj, 630, &DateTimePKData{}, time.Date(2006, time.June, 16, 10, 42, 37, 0, time.UTC)},
	{pkObj, 640, &EmailPKData{}, "hovercraft@example.com"},
	// Row 660 (FilePathPKData) is in unsupportedSerializerTestData.
	{pkObj, 670, &DecimalPKData{}, orm.MustDecimal("12.345")},
	{pkObj, 671, &DecimalPKData{}, orm.MustDecimal("-12.345")},
	{pkObj, 672, &DecimalPKData{}, orm.MustDecimal("0.0")},
	{pkObj, 673, &FloatPKData{}, 12.345},
	{pkObj, 674, &FloatPKData{}, -12.345},
	{pkObj, 675, &FloatPKData{}, 0.0},
	{pkObj, 680, &IntegerPKData{}, int64(123456789)},
	{pkObj, 681, &IntegerPKData{}, int64(-123456789)},
	// Row 682 (IntegerPKData, 0) is in unsupportedSerializerTestData.
	{pkObj, 695, &GenericIPAddressPKData{}, "fe80:1424:2223:6cff:fe8a:2e8a:2151:abcd"},
	{pkObj, 720, &PositiveIntegerPKData{}, int64(123456789)},
	{pkObj, 730, &PositiveSmallIntegerPKData{}, int16(12)},
	{pkObj, 740, &SlugPKData{}, "this-is-a-slug"},
	{pkObj, 750, &SmallPKData{}, int16(12)},
	{pkObj, 751, &SmallPKData{}, int16(-12)},
	{pkObj, 752, &SmallPKData{}, int16(0)},
	{pkObj, 770, &TimePKData{}, orm.NewTime(10, 42, 37, 0)},
	{pkObj, 791, &UUIDData{}, dataUUIDObj},
	// Row 792 (FKToUUID) is in unsupportedSerializerTestData.
	{pkObj, 793, &UUIDDefaultData{}, dataUUIDObj},
	{dataObj, 800, &AutoNowDateTimeData{}, time.Date(2006, time.June, 16, 10, 42, 37, 0, time.UTC)},
	{dataObj, 810, &ModifyingSaveData{}, int64(42)},
	{inheritedObj, 900, &InheritAbstractModel{}, inheritedData{childData: 37, parentData: 42}},
	// Rows 910 (ExplicitInheritBaseModel) and 920 (InheritBaseModel) are in unsupportedSerializerTestData.
	{dataObj, 1000, &BigIntegerData{}, int64(9223372036854775807)},
	{dataObj, 1001, &BigIntegerData{}, int64(-9223372036854775808)},
	{dataObj, 1002, &BigIntegerData{}, int64(0)},
	{dataObj, 1003, &BigIntegerData{}, nil},
	// No Go counterpart: (data_obj, 1004, LengthModel, 0): tests Python's __len__ == 0 truthiness on a model instance ("if related:" vs "if related is not None:").
	// No Go counterpart: (data_obj, 1005, LengthModel, 1): companion row to PK=1004 for Python's __len__ truthiness on LengthModel.
}

// unsupportedSerializerTestData holds the rows from Django's test_data that
// need features not implemented in djangogo.
var unsupportedSerializerTestData = []unsupportedDataTestRow{
	{feature: "FileField", helper: "data_obj", pk: 50, modelName: "FileData", data: "file:///foo/bar/whiz.txt"},
	{feature: "FileField", helper: "data_obj", pk: 52, modelName: "FileData", data: ""},
	{feature: "FilePathField", helper: "data_obj", pk: 60, modelName: "FilePathData", data: "/foo/bar/whiz.txt"},
	{feature: "FilePathField", helper: "data_obj", pk: 62, modelName: "FilePathData", data: ""},
	{feature: "ForeignKey to_field and non-integer target", helper: "fk_obj", pk: 450, modelName: "FKDataToField", data: "UAnchor 1"},
	{feature: "ForeignKey to_field and non-integer target", helper: "fk_obj", pk: 451, modelName: "FKDataToField", data: "UAnchor 2"},
	{feature: "ForeignKey to_field and non-integer target", helper: "fk_obj", pk: 452, modelName: "FKDataToField", data: nil},
	{feature: "FilePathField", helper: "pk_obj", pk: 660, modelName: "FilePathPKData", data: "/foo/bar/whiz.txt"},
	{feature: "ForeignKey to non-integer primary key", helper: "fk_obj", pk: 792, modelName: "FKToUUID", data: dataUUIDObj},
	{feature: "multi-table model inheritance", helper: "inherited_obj", pk: 910, modelName: "ExplicitInheritBaseModel", data: inheritedData{childData: 37, parentData: 42}},
	{feature: "multi-table model inheritance", helper: "inherited_obj", pk: 920, modelName: "InheritBaseModel", data: inheritedData{childData: 37, parentData: 42}},
	{feature: "ImageField", helper: "data_obj", pk: 86, modelName: "ImageData", data: "file:///foo/bar/whiz.png"},
	{feature: "ImageField", helper: "data_obj", pk: 88, modelName: "ImageData", data: ""},
	{feature: "an integer primary key that is not an auto field (an int64 pk is always auto, and an auto key of 0 counts as unset)", helper: "pk_obj", pk: 682, modelName: "IntegerPKData", data: int64(0)},
}

// assertSerializer ports Django's tests/serializers/test_data.py::assert_serializer.
// It creates every row in data inside an atomic transaction (so forward FK and
// M2M references defer constraint checks until commit), serializes all created
// objects plus all Tag rows with indent=2, deserializes and saves every object,
// compares every row in a subtest, and verifies that no model gained rows.
func assertSerializer(t *testing.T, db *orm.DB, format string, data []dataTestRow) {
	t.Helper()
	ctx := context.Background()

	var objects []any
	err := db.Atomic(ctx, func(txCtx context.Context) error {
		for _, row := range data {
			created := row.helper.createObject(txCtx, t, db, row.pk, row.model, row.data)
			objects = append(objects, created...)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create objects Atomic: %v", err)
	}

	type modelCount struct {
		model *orm.Model
		count int64
	}
	seenModels := make(map[*orm.Model]bool)
	var instanceCounts []modelCount
	for _, row := range data {
		m, ok := db.Registry().ModelOf(row.model)
		if !ok {
			t.Fatalf("model %T is not registered", row.model)
		}
		if seenModels[m] {
			continue
		}
		seenModels[m] = true
		c, err := orm.QueryModel(db, m).Count(ctx)
		if err != nil {
			t.Fatalf("Count(%s): %v", m.Name(), err)
		}
		instanceCounts = append(instanceCounts, modelCount{model: m, count: c})
	}

	tags, err := orm.Query[Tag](db).All(ctx)
	if err != nil {
		t.Fatalf("Query[Tag].All: %v", err)
	}
	for i := range tags {
		objects = append(objects, &tags[i])
	}

	serializedData, err := serializers.Serialize(ctx, db, format, objects, serializers.Indent(2))
	if err != nil {
		t.Fatalf("Serialize(%s): %v", format, err)
	}

	err = db.Atomic(ctx, func(txCtx context.Context) error {
		for obj, err := range serializers.Deserialize(txCtx, db, format, strings.NewReader(serializedData)) {
			if err != nil {
				return err
			}
			if err := obj.Save(txCtx, db); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Deserialize/Save(%s): %v", format, err)
	}

	for _, row := range data {
		m, _ := db.Registry().ModelOf(row.model)
		subName := fmt.Sprintf("model=%s/pk=%d/data=%s", m.Name(), row.pk, validators.PyString(row.data))
		t.Run(subName, func(t *testing.T) {
			row.helper.compareObject(ctx, t, db, row.pk, row.model, row.data)
		})
	}

	for _, mc := range instanceCounts {
		subName := fmt.Sprintf("count/model=%s", mc.model.Name())
		t.Run(subName, func(t *testing.T) {
			gotCount, err := orm.QueryModel(db, mc.model).Count(ctx)
			if err != nil {
				t.Fatalf("Count(%s): %v", mc.model.Name(), err)
			}
			djangotest.AssertEqual(t, gotCount, mc.count)
		})
	}
}

// Ported from Django: tests/serializers/test_data.py::serializerTest
// Check that every supported field kind and relation kind round-trips through each serializer format.
func TestSerializerDataSerializer(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				assertSerializer(t, db, format, serializerTestData)
			})
		})
	}
}

// Ported from Django: tests/serializers/test_data.py::serializerTest
// Check that the Django test_data rows that need unimplemented features are recorded and skipped.
func TestSerializerDataSerializerUnsupported(t *testing.T) {
	for _, row := range unsupportedSerializerTestData {
		subName := fmt.Sprintf("%s/pk=%v", row.modelName, row.pk)
		t.Run(subName, func(t *testing.T) {
			t.Skipf("FIXME-PORT: Go: %s, Django: (%s, %v, %s, %v)", row.feature, row.helper, row.pk, row.modelName, row.data)
		})
	}
}

// Ported from Django: tests/serializers/test_data.py::serializerTestPK0
// Check that an object with an explicit auto primary key of 0 and a foreign key pointing to it round-trip.
func TestSerializerDataSerializerPK0(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				if db.Dialect().Name() == "postgres" {
					t.Skip("PostgreSQL does not allow auto primary key 0 (allows_auto_pk_0 is false)")
				}
				t.Skip("FIXME-PORT: Go: orm.SaveRaw and orm.FK treat integer primary key 0 as unset, Django: serializerTestPK0 round-trips Anchor(pk=0) and FKData(data_id=0)")
			})
		})
	}
}

// Ported from Django: tests/serializers/test_data.py::serializerTestNullValueStingField
// Check that null values on nullable binary and string-backed fields round-trip as nil.
func TestSerializerDataSerializerNullValueStringField(t *testing.T) {
	data := []dataTestRow{
		{dataObj, 1, &BinaryData{}, nil},
		{dataObj, 2, &CharData{}, nil},
		{dataObj, 3, &EmailData{}, nil},
		// Row (data_obj, 4, FilePathData, None) is skipped because FilePathField is not implemented.
		{dataObj, 5, &SlugData{}, nil},
		{dataObj, 6, &TextData{}, nil},
	}
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				assertSerializer(t, db, format, data)
			})
		})
	}
}

// Ported from Django: tests/serializers/test_data.py::serializerTestTextFieldPK
// Check that a model using a TextField with line breaks as its primary key round-trips through each serializer format.
func TestSerializerDataSerializerTextFieldPK(t *testing.T) {
	data := []dataTestRow{
		{
			pkObj,
			1,
			&TextPKData{},
			`This is a long piece of text.
            It contains line breaks.
            Several of them.
            The end.`,
		},
	}
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				assertSerializer(t, db, format, data)
			})
		})
	}
}
