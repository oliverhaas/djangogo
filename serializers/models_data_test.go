package serializers_test

import (
	"time"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/orm"
)

// Data models from Django's tests/serializers/models/data.py for basic field
// marshalling, including NULL values where allowed.

// BinaryData is Django's serializers.BinaryData.
type BinaryData struct {
	ID   int64
	Data []byte `orm:"null"`
}

// BooleanData is Django's serializers.BooleanData.
type BooleanData struct {
	ID   int64
	Data *bool `orm:"default=false;null"`
}

// CharData is Django's serializers.CharData.
type CharData struct {
	ID   int64
	Data *string `orm:"max_length=30;null"`
}

// DateData is Django's serializers.DateData.
type DateData struct {
	ID   int64
	Data *orm.Date `orm:"null"`
}

// DateTimeData is Django's serializers.DateTimeData.
type DateTimeData struct {
	ID   int64
	Data *time.Time `orm:"null"`
}

// DecimalData is Django's serializers.DecimalData.
type DecimalData struct {
	ID   int64
	Data *orm.Decimal `orm:"null;decimal_places=3;max_digits=5"`
}

// EmailData is Django's serializers.EmailData.
type EmailData struct {
	ID   int64
	Data *string `orm:"type=email;null"`
}

// FloatData is Django's serializers.FloatData.
type FloatData struct {
	ID   int64
	Data *float64 `orm:"null"`
}

// IntegerData is Django's serializers.IntegerData.
type IntegerData struct {
	ID   int64
	Data *int64 `orm:"null"`
}

// BigIntegerData is Django's serializers.BigIntegerData.
type BigIntegerData struct {
	ID   int64
	Data *int64 `orm:"type=bigint;null"`
}

// GenericIPAddressData is Django's serializers.GenericIPAddressData.
type GenericIPAddressData struct {
	ID   int64
	Data *string `orm:"type=ip;null"`
}

// PositiveBigIntegerData is Django's serializers.PositiveBigIntegerData.
type PositiveBigIntegerData struct {
	ID   int64
	Data *uint64 `orm:"type=positive_bigint;null"`
}

// PositiveIntegerData is Django's serializers.PositiveIntegerData.
type PositiveIntegerData struct {
	ID   int64
	Data *uint32 `orm:"type=positive_int;null"`
}

// PositiveSmallIntegerData is Django's serializers.PositiveSmallIntegerData.
type PositiveSmallIntegerData struct {
	ID   int64
	Data *uint16 `orm:"type=positive_smallint;null"`
}

// SlugData is Django's serializers.SlugData.
type SlugData struct {
	ID   int64
	Data *string `orm:"type=slug;null"`
}

// SmallData is Django's serializers.SmallData.
type SmallData struct {
	ID   int64
	Data *int16 `orm:"type=smallint;null"`
}

// TextData is Django's serializers.TextData.
type TextData struct {
	ID   int64
	Data *string `orm:"type=text;null"`
}

// TimeData is Django's serializers.TimeData.
type TimeData struct {
	ID   int64
	Data *orm.Time `orm:"null"`
}

// Tag is Django's serializers.Tag: a generic tag on an item.
type Tag struct {
	orm.RelCache
	ID            int64
	Data          string                           `orm:"type=slug"`
	ContentType   orm.FK[contenttypes.ContentType] `orm:"on_delete=cascade"`
	ObjectID      int64                            `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
}

// Meta orders Tag rows by data.
func (Tag) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"data"}}
}

// GenericData is Django's serializers.GenericData.
type GenericData struct {
	orm.RelCache
	ID   int64
	Data string `orm:"max_length=30"`
	Tags contenttypes.GenericRelation[Tag]
}

// UniqueAnchor is Django's serializers.UniqueAnchor.
type UniqueAnchor struct {
	ID   int64
	Data string `orm:"unique;max_length=30"`
}

// FKData is Django's serializers.FKData.
type FKData struct {
	orm.RelCache
	ID   int64
	Data orm.FK[Anchor] `orm:"null;on_delete=set_null"`
}

// O2OData is Django's serializers.O2OData.
type O2OData struct {
	orm.RelCache
	Data orm.O2O[Anchor] `orm:"pk;on_delete=cascade"`
}

// FKSelfData is Django's serializers.FKSelfData.
type FKSelfData struct {
	orm.RelCache
	ID   int64
	Data orm.FK[FKSelfData] `orm:"null;on_delete=cascade"`
}

// M2MSelfData is Django's serializers.M2MSelfData.
type M2MSelfData struct {
	orm.RelCache
	ID   int64
	Data orm.M2M[M2MSelfData] `orm:"symmetrical=false"`
}

// FKDataToO2O is Django's serializers.FKDataToO2O.
type FKDataToO2O struct {
	orm.RelCache
	ID   int64
	Data orm.FK[O2OData] `orm:"null;on_delete=set_null"`
}

// M2MIntermediateData is Django's serializers.M2MIntermediateData.
type M2MIntermediateData struct {
	orm.RelCache
	ID   int64
	Data orm.M2M[Anchor] `orm:"through=Intermediate"`
}

// Intermediate is Django's serializers.Intermediate.
type Intermediate struct {
	orm.RelCache
	ID    int64
	Left  orm.FK[M2MIntermediateData] `orm:"on_delete=cascade"`
	Right orm.FK[Anchor]              `orm:"on_delete=cascade"`
	Extra string                      `orm:"max_length=30;blank;default=doesn't matter"`
}

// Primary-key models from Django's tests/serializers/models/data.py for
// deserialization of user-defined primary key fields.

// BooleanPKData is Django's serializers.BooleanPKData.
type BooleanPKData struct {
	Data bool `orm:"pk;default=false"`
}

// CharPKData is Django's serializers.CharPKData.
type CharPKData struct {
	Data string `orm:"max_length=30;pk"`
}

// DatePKData is Django's serializers.DatePKData.
type DatePKData struct {
	Data orm.Date `orm:"pk"`
}

// DateTimePKData is Django's serializers.DateTimePKData.
type DateTimePKData struct {
	Data time.Time `orm:"pk"`
}

// DecimalPKData is Django's serializers.DecimalPKData.
type DecimalPKData struct {
	Data orm.Decimal `orm:"pk;decimal_places=3;max_digits=5"`
}

// EmailPKData is Django's serializers.EmailPKData.
type EmailPKData struct {
	Data string `orm:"type=email;pk"`
}

// FloatPKData is Django's serializers.FloatPKData.
type FloatPKData struct {
	Data float64 `orm:"pk"`
}

// IntegerPKData is Django's serializers.IntegerPKData.
type IntegerPKData struct {
	Data int64 `orm:"pk"`
}

// GenericIPAddressPKData is Django's serializers.GenericIPAddressPKData.
type GenericIPAddressPKData struct {
	Data string `orm:"type=ip;pk"`
}

// PositiveIntegerPKData is Django's serializers.PositiveIntegerPKData.
type PositiveIntegerPKData struct {
	Data uint32 `orm:"type=positive_int;pk"`
}

// PositiveSmallIntegerPKData is Django's serializers.PositiveSmallIntegerPKData.
type PositiveSmallIntegerPKData struct {
	Data uint16 `orm:"type=positive_smallint;pk"`
}

// SlugPKData is Django's serializers.SlugPKData.
type SlugPKData struct {
	Data string `orm:"type=slug;pk"`
}

// SmallPKData is Django's serializers.SmallPKData.
type SmallPKData struct {
	Data int16 `orm:"type=smallint;pk"`
}

// TextPKData is Django's serializers.TextPKData.
type TextPKData struct {
	Data string `orm:"type=text;pk"`
}

// TimePKData is Django's serializers.TimePKData.
type TimePKData struct {
	Data orm.Time `orm:"pk"`
}

// UUIDData is Django's serializers.UUIDData.
type UUIDData struct {
	Data orm.UUID `orm:"pk"`
}

// UUIDDefaultData is Django's serializers.UUIDDefaultData.
type UUIDDefaultData struct {
	Data orm.UUID `orm:"pk;default=uuid4"`
}

// AutoNowDateTimeData is Django's serializers.AutoNowDateTimeData.
type AutoNowDateTimeData struct {
	ID   int64
	Data *time.Time `orm:"null;auto_now"`
}

// ModifyingSaveData is Django's serializers.ModifyingSaveData.
type ModifyingSaveData struct {
	ID   int64
	Data *int64 `orm:"null"`
}

// InheritAbstractModel is Django's serializers.InheritAbstractModel, with
// AbstractBaseModel's parent_data field included on the concrete struct.
type InheritAbstractModel struct {
	ID         int64
	ParentData int64
	ChildData  int64
}

// serializerDataModels returns the data-test models registered in serializerModels.
func serializerDataModels() []any {
	return []any{
		&BinaryData{},
		&BooleanData{},
		&CharData{},
		&DateData{},
		&DateTimeData{},
		&DecimalData{},
		&EmailData{},
		&FloatData{},
		&IntegerData{},
		&BigIntegerData{},
		&GenericIPAddressData{},
		&PositiveBigIntegerData{},
		&PositiveIntegerData{},
		&PositiveSmallIntegerData{},
		&SlugData{},
		&SmallData{},
		&TextData{},
		&TimeData{},
		&Tag{},
		&GenericData{},
		&UniqueAnchor{},
		&FKData{},
		&O2OData{},
		&FKSelfData{},
		&M2MSelfData{},
		&FKDataToO2O{},
		&M2MIntermediateData{},
		&Intermediate{},
		&BooleanPKData{},
		&CharPKData{},
		&DatePKData{},
		&DateTimePKData{},
		&DecimalPKData{},
		&EmailPKData{},
		&FloatPKData{},
		&IntegerPKData{},
		&GenericIPAddressPKData{},
		&PositiveIntegerPKData{},
		&PositiveSmallIntegerPKData{},
		&SlugPKData{},
		&SmallPKData{},
		&TextPKData{},
		&TimePKData{},
		&UUIDData{},
		&UUIDDefaultData{},
		&AutoNowDateTimeData{},
		&ModifyingSaveData{},
		&InheritAbstractModel{},
	}
}
