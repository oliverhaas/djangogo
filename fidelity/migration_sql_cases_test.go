package fidelity

import (
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// migrationCase defines one canonical Go migration scenario matching an entry
// in fidelity/oracle/migration_sql.py. Setup holds prerequisite operations
// applied before Target; Target holds the migration whose sqlmigrate output is
// compared against Django.
type migrationCase struct {
	name   string
	setup  []migrations.Operation
	target []migrations.Operation
}

// canonicalMigrationCases returns the canonical table of migration SQL
// differential cases, in the same order as fidelity/oracle/migration_sql.py.
func canonicalMigrationCases() []migrationCase {
	idField := migrations.FieldState{
		Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true,
	}

	tagOp := migrations.CreateModel{
		Name: "Tag",
		Fields: []migrations.FieldState{
			idField,
			{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
			{Name: "Priority", Column: "priority", Kind: orm.KindSmallInt},
			{Name: "Active", Column: "active", Kind: orm.KindBool},
			{Name: "Notes", Column: "notes", Kind: orm.KindChar, MaxLength: 200, Null: true},
			{Name: "Code", Column: "code", Kind: orm.KindChar, MaxLength: 20, Unique: true},
		},
	}

	allKindsOp := migrations.CreateModel{
		Name: "AllKinds",
		Fields: []migrations.FieldState{
			idField,
			{Name: "IntVal", Column: "int_val", Kind: orm.KindInt},
			{Name: "SmallInt", Column: "small_int", Kind: orm.KindSmallInt},
			{Name: "BigInt", Column: "big_int", Kind: orm.KindBigInt},
			{Name: "PosSmallInt", Column: "pos_small_int", Kind: orm.KindPositiveSmallInt},
			{Name: "PosInt", Column: "pos_int", Kind: orm.KindPositiveInt},
			{Name: "PosBigInt", Column: "pos_big_int", Kind: orm.KindPositiveBigInt},
			{Name: "FloatVal", Column: "float_val", Kind: orm.KindFloat},
			{Name: "DecimalVal", Column: "decimal_val", Kind: orm.KindDecimal, MaxDigits: 10, DecimalPlaces: 2},
			{Name: "CharVal", Column: "char_val", Kind: orm.KindChar, MaxLength: 100},
			{Name: "TextVal", Column: "text_val", Kind: orm.KindText},
			{Name: "SlugVal", Column: "slug_val", Kind: orm.KindSlug, MaxLength: 50, DBIndex: true},
			{Name: "EmailVal", Column: "email_val", Kind: orm.KindEmail, MaxLength: 254},
			{Name: "URLVal", Column: "url_val", Kind: orm.KindURL, MaxLength: 200},
			{Name: "IPVal", Column: "ip_val", Kind: orm.KindGenericIPAddress},
			{Name: "BoolVal", Column: "bool_val", Kind: orm.KindBool},
			{Name: "DatetimeVal", Column: "datetime_val", Kind: orm.KindDateTime},
			{Name: "DateVal", Column: "date_val", Kind: orm.KindDate},
			{Name: "TimeVal", Column: "time_val", Kind: orm.KindTime},
			{Name: "DurationVal", Column: "duration_val", Kind: orm.KindDuration},
			{Name: "UUIDVal", Column: "uuid_val", Kind: orm.KindUUID},
			{Name: "JSONVal", Column: "json_val", Kind: orm.KindJSON},
			{Name: "BinaryVal", Column: "binary_val", Kind: orm.KindBinary},
		},
	}

	authorOp := migrations.CreateModel{
		Name: "Author",
		Fields: []migrations.FieldState{
			idField,
			{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
			{Name: "Code", Column: "code", Kind: orm.KindChar, MaxLength: 20, Unique: true},
		},
	}

	authorFK := migrations.FieldState{
		Name:        "Author",
		Column:      "author_id",
		Kind:        orm.KindBigInt,
		RelKind:     orm.RelFK,
		RelTo:       "migfid.Author",
		RelOnDelete: orm.OnDeleteCascade,
	}

	editorFK := migrations.FieldState{
		Name:        "Editor",
		Column:      "editor_id",
		Kind:        orm.KindBigInt,
		Null:        true,
		RelKind:     orm.RelFK,
		RelTo:       "migfid.Author",
		RelOnDelete: orm.OnDeleteSetNull,
	}

	articleOp := migrations.CreateModel{
		Name: "Article",
		Fields: []migrations.FieldState{
			idField,
			{Name: "Title", Column: "title", Kind: orm.KindChar, MaxLength: 100},
			{Name: "Rank", Column: "rank", Kind: orm.KindSmallInt},
			{Name: "Summary", Column: "summary", Kind: orm.KindChar, MaxLength: 200, Null: true},
			authorFK,
		},
	}

	articleMetaOp := migrations.CreateModel{
		Name: "Article",
		Fields: []migrations.FieldState{
			idField,
			{Name: "Title", Column: "title", Kind: orm.KindChar, MaxLength: 100},
			{Name: "CreatedAt", Column: "created_at", Kind: orm.KindDateTime},
			authorFK,
			editorFK,
		},
		Indexes: []migrations.Index{
			{Name: "migfid_art_title_created_idx", Fields: []string{"Title", "-CreatedAt"}},
		},
		UniqueTogether: [][]string{{"Author", "Title"}},
	}

	titleRankIndex := migrations.Index{
		Name:   "migfid_art_title_rank_idx",
		Fields: []string{"Title", "-Rank"},
	}

	return []migrationCase{
		// --- CreateModel / DeleteModel ---
		{
			name:   "create_model_basic",
			target: []migrations.Operation{tagOp},
		},
		{
			name:   "create_model_all_kinds",
			target: []migrations.Operation{allKindsOp},
		},
		{
			name:   "create_model_fk_and_meta",
			setup:  []migrations.Operation{authorOp},
			target: []migrations.Operation{articleMetaOp},
		},
		{
			name:   "delete_model",
			setup:  []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{migrations.DeleteModel{Name: "Article"}},
		},
		// --- AddField ---
		{
			name:  "add_field_nullable",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AddField{
					Model: "Article",
					Field: migrations.FieldState{
						Name: "Subtitle", Column: "subtitle", Kind: orm.KindChar, MaxLength: 120, Null: true,
					},
				},
			},
		},
		{
			name:  "add_field_default_int",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AddField{
					Model:   "Article",
					Field:   migrations.FieldState{Name: "Rating", Column: "rating", Kind: orm.KindInt},
					Default: int64(5),
				},
			},
		},
		{
			name:  "add_field_default_char",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AddField{
					Model:   "Article",
					Field:   migrations.FieldState{Name: "Status", Column: "status", Kind: orm.KindChar, MaxLength: 20},
					Default: "draft",
				},
			},
		},
		{
			name:  "add_field_default_bool",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AddField{
					Model:   "Article",
					Field:   migrations.FieldState{Name: "Featured", Column: "featured", Kind: orm.KindBool},
					Default: true,
				},
			},
		},
		{
			name:  "add_field_fk",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AddField{
					Model: "Article",
					Field: migrations.FieldState{
						Name:        "Reviewer",
						Column:      "reviewer_id",
						Kind:        orm.KindBigInt,
						Null:        true,
						RelKind:     orm.RelFK,
						RelTo:       "migfid.Author",
						RelOnDelete: orm.OnDeleteSetNull,
					},
				},
			},
		},
		// --- AlterField ---
		{
			name:  "alter_field_varchar_len",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AlterField{
					Model: "Article",
					Field: migrations.FieldState{
						Name: "Title", Column: "title", Kind: orm.KindChar, MaxLength: 250,
					},
				},
			},
		},
		{
			name:  "alter_field_int_type",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AlterField{
					Model: "Article",
					Field: migrations.FieldState{
						Name: "Rank", Column: "rank", Kind: orm.KindBigInt,
					},
				},
			},
		},
		{
			name:  "alter_field_null_to_not_null",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AlterField{
					Model:   "Article",
					Field:   migrations.FieldState{Name: "Summary", Column: "summary", Kind: orm.KindChar, MaxLength: 200},
					Default: "none",
				},
			},
		},
		{
			name:  "alter_field_not_null_to_null",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AlterField{
					Model: "Article",
					Field: migrations.FieldState{Name: "Rank", Column: "rank", Kind: orm.KindSmallInt, Null: true},
				},
			},
		},
		{
			name:  "alter_field_add_unique",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AlterField{
					Model: "Article",
					Field: migrations.FieldState{Name: "Title", Column: "title", Kind: orm.KindChar, MaxLength: 100, Unique: true},
				},
			},
		},
		{
			name:  "alter_field_remove_unique",
			setup: []migrations.Operation{authorOp},
			target: []migrations.Operation{
				migrations.AlterField{
					Model: "Author",
					Field: migrations.FieldState{Name: "Code", Column: "code", Kind: orm.KindChar, MaxLength: 20},
				},
			},
		},
		{
			name:  "alter_field_add_index",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AlterField{
					Model: "Article",
					Field: migrations.FieldState{Name: "Rank", Column: "rank", Kind: orm.KindSmallInt, DBIndex: true},
				},
			},
		},
		// --- RemoveField / RenameField ---
		{
			name:   "remove_field",
			setup:  []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{migrations.RemoveField{Model: "Article", Field: "Rank"}},
		},
		{
			name:   "rename_field",
			setup:  []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{migrations.RenameField{Model: "Article", OldName: "Title", NewName: "Headline"}},
		},
		// --- RenameModel / AlterModelTable ---
		{
			name:   "rename_model",
			setup:  []migrations.Operation{tagOp},
			target: []migrations.Operation{migrations.RenameModel{OldName: "Tag", NewName: "Label"}},
		},
		{
			name:   "rename_model_with_fk",
			setup:  []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{migrations.RenameModel{OldName: "Article", NewName: "Post"}},
		},
		{
			name:   "alter_model_table",
			setup:  []migrations.Operation{tagOp},
			target: []migrations.Operation{migrations.AlterModelTable{Name: "Tag", Table: "custom_tags"}},
		},
		// --- AddIndex / RemoveIndex / AlterUniqueTogether ---
		{
			name:   "add_index",
			setup:  []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{migrations.AddIndex{Model: "Article", Index: titleRankIndex}},
		},
		{
			name: "remove_index",
			setup: []migrations.Operation{
				authorOp,
				articleOp,
				migrations.AddIndex{Model: "Article", Index: titleRankIndex},
			},
			target: []migrations.Operation{
				migrations.RemoveIndex{Model: "Article", Name: "migfid_art_title_rank_idx"},
			},
		},
		{
			name:  "alter_unique_together_add",
			setup: []migrations.Operation{authorOp, articleOp},
			target: []migrations.Operation{
				migrations.AlterUniqueTogether{Name: "Article", UniqueTogether: [][]string{{"Author", "Title"}}},
			},
		},
		{
			name: "alter_unique_together_remove",
			setup: []migrations.Operation{
				authorOp,
				articleOp,
				migrations.AlterUniqueTogether{Name: "Article", UniqueTogether: [][]string{{"Author", "Title"}}},
			},
			target: []migrations.Operation{
				migrations.AlterUniqueTogether{Name: "Article", UniqueTogether: nil},
			},
		},
	}
}
