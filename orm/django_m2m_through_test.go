package orm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/m2m_through/models.py.

type m2mtPerson struct {
	ID   int64
	Name string `orm:"max_length=128"`
}

// Meta sets default name ordering as in Django's m2m_through.Person.
func (m2mtPerson) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

type m2mtGroup struct {
	ID                int64
	Name              string              `orm:"max_length=128"`
	Members           orm.M2M[m2mtPerson] `orm:"through=m2mtMembership"`
	CustomMembers     orm.M2M[m2mtPerson] `orm:"through=m2mtCustomMembership;related_name=custom"`
	NoDefaultsNoNulls orm.M2M[m2mtPerson] `orm:"through=m2mtNoDefaultsOrNulls;related_name=testnodefaultsnonulls"`
}

// Meta sets default name ordering as in Django's m2m_through.Group.
func (m2mtGroup) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

type m2mtMembership struct {
	ID           int64
	Person       orm.FK[m2mtPerson] `orm:"on_delete=cascade"`
	Group        orm.FK[m2mtGroup]  `orm:"on_delete=cascade"`
	DateJoined   time.Time          `orm:"auto_now_add"`
	InviteReason string             `orm:"max_length=64;null"`
}

// Meta sets default ordering as in Django's m2m_through.Membership.
func (m2mtMembership) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"date_joined", "invite_reason", "group"}}
}

type m2mtCustomMembership struct {
	ID         int64
	Person     orm.FK[m2mtPerson]     `orm:"on_delete=cascade;column=custom_person_column;related_name=custom_person_related_name"`
	Group      orm.FK[m2mtGroup]      `orm:"on_delete=cascade"`
	WeirdFK    orm.FK[m2mtMembership] `orm:"null;on_delete=set_null"`
	DateJoined time.Time              `orm:"auto_now_add"`
}

// Meta sets custom table and ordering as in Django's m2m_through.CustomMembership.
func (m2mtCustomMembership) Meta() orm.Meta {
	return orm.Meta{Table: "test_m2mt_table", Ordering: []string{"date_joined"}}
}

type m2mtNoDefaultsOrNulls struct {
	ID              int64
	Person          orm.FK[m2mtPerson] `orm:"on_delete=cascade"`
	Group           orm.FK[m2mtGroup]  `orm:"on_delete=cascade"`
	NoDefaultNoNull int64
}

type m2mtPersonSelfRef struct {
	ID         int64
	Name       string                     `orm:"max_length=5"`
	Friends    orm.M2M[m2mtPersonSelfRef] `orm:"through=m2mtFriendship;symmetrical=false"`
	SymFriends orm.M2M[m2mtPersonSelfRef] `orm:"through=m2mtSymmetricalFriendship;symmetrical=true"`
}

type m2mtFriendship struct {
	ID           int64
	First        orm.FK[m2mtPersonSelfRef] `orm:"on_delete=cascade;related_name=rel_from_set"`
	Second       orm.FK[m2mtPersonSelfRef] `orm:"on_delete=cascade;related_name=rel_to_set"`
	DateFriended time.Time                 `orm:"auto_now_add"`
}

type m2mtSymmetricalFriendship struct {
	ID           int64
	First        orm.FK[m2mtPersonSelfRef] `orm:"on_delete=cascade"`
	Second       orm.FK[m2mtPersonSelfRef] `orm:"on_delete=cascade;related_name=+"`
	DateFriended orm.Date                  `orm:"type=date"`
}

type m2mtEvent struct {
	ID       int64
	Title    string              `orm:"max_length=50"`
	Invitees orm.M2M[m2mtPerson] `orm:"through=m2mtInvitation;through_fields=event,invitee;related_name=events_invited"`
}

type m2mtInvitation struct {
	ID      int64
	Event   orm.FK[m2mtEvent]  `orm:"on_delete=cascade;related_name=invitations"`
	Inviter orm.FK[m2mtPerson] `orm:"on_delete=cascade;related_name=invitations_sent"`
	Invitee orm.FK[m2mtPerson] `orm:"on_delete=cascade;related_name=invitations"`
}

type m2mtEmployee struct {
	ID           int64
	Name         string                `orm:"max_length=5"`
	Subordinates orm.M2M[m2mtEmployee] `orm:"through=m2mtRelationship;through_fields=source,target;symmetrical=false"`
}

// Meta sets pk ordering as in Django's m2m_through.Employee.
func (m2mtEmployee) Meta() orm.Meta { return orm.Meta{Ordering: []string{"pk"}} }

type m2mtRelationship struct {
	ID      int64
	Another orm.FK[m2mtEmployee] `orm:"null;on_delete=set_null;related_name=rel_another_set"`
	Target  orm.FK[m2mtEmployee] `orm:"on_delete=cascade;related_name=rel_target_set"`
	Source  orm.FK[m2mtEmployee] `orm:"on_delete=cascade;related_name=rel_source_set"`
}

// m2mtModels returns the model list for the m2m_through test app.
func m2mtModels() []any {
	return []any{djangotest.App("m2m_through",
		&m2mtPerson{}, &m2mtGroup{}, &m2mtMembership{}, &m2mtCustomMembership{},
		&m2mtNoDefaultsOrNulls{}, &m2mtPersonSelfRef{}, &m2mtFriendship{},
		&m2mtSymmetricalFriendship{}, &m2mtEvent{}, &m2mtInvitation{},
		&m2mtEmployee{}, &m2mtRelationship{},
	)}
}

// m2mtFixture holds the base people and groups for M2mThroughTests.
type m2mtFixture struct {
	bob, jim, jane *m2mtPerson
	rock, roll     *m2mtGroup
}

// m2mtSetUp creates the base people and groups.
func m2mtSetUp(t *testing.T, db *orm.DB) m2mtFixture {
	t.Helper()
	return m2mtFixture{
		bob:  create(t, db, &m2mtPerson{Name: "Bob"}),
		jim:  create(t, db, &m2mtPerson{Name: "Jim"}),
		jane: create(t, db, &m2mtPerson{Name: "Jane"}),
		rock: create(t, db, &m2mtGroup{Name: "Rock"}),
		roll: create(t, db, &m2mtGroup{Name: "Roll"}),
	}
}

// personNames extracts Name values from m2mtPerson slices.
func personNames(ps []m2mtPerson) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}

// groupNames extracts Name values from m2mtGroup slices.
func groupNames(gs []m2mtGroup) []string {
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.Name
	}
	return out
}

// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_retrieve_intermediate_items
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_retrieve_reverse_intermediate_items
func TestM2MThroughRetrieveItems(t *testing.T) {
	djangotest.EachDB(t, m2mtModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mtSetUp(t, db)

		create(t, db, &m2mtMembership{Person: fkTo[m2mtPerson](f.jim.ID), Group: fkTo[m2mtGroup](f.rock.ID)})
		create(t, db, &m2mtMembership{Person: fkTo[m2mtPerson](f.jane.ID), Group: fkTo[m2mtGroup](f.rock.ID)})
		create(t, db, &m2mtMembership{Person: fkTo[m2mtPerson](f.jim.ID), Group: fkTo[m2mtGroup](f.roll.ID)})

		members, err := orm.Many[m2mtPerson](db, f.rock, "members").All(ctx)
		if err != nil {
			t.Fatalf("rock.members: %v", err)
		}
		djangotest.AssertSequenceEqual(t, personNames(members), []string{"Jane", "Jim"})

		groups, err := orm.Many[m2mtGroup](db, f.jim, "m2mtgroup_set").All(ctx)
		if err != nil {
			t.Fatalf("jim.m2mtgroup_set: %v", err)
		}
		djangotest.AssertSequenceEqual(t, groupNames(groups), []string{"Rock", "Roll"})
	})
}

// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_add_on_m2m_with_intermediate_model
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_add_on_m2m_with_intermediate_model_callable_through_default
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_add_on_m2m_with_intermediate_model_value_required
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_add_on_m2m_with_intermediate_model_value_required_fails
func TestM2MThroughAddAndDefaults(t *testing.T) {
	djangotest.EachDB(t, m2mtModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mtSetUp(t, db)

		// Add with through_defaults.
		if err := orm.Many[m2mtPerson](db, f.rock, "members").AddWith(ctx, []*m2mtPerson{f.bob},
			orm.ThroughDefaults(orm.Defaults{"invite_reason": "He is good."}),
		); err != nil {
			t.Fatalf("AddWith: %v", err)
		}
		members, err := orm.Many[m2mtPerson](db, f.rock, "members").All(ctx)
		if err != nil {
			t.Fatalf("members: %v", err)
		}
		djangotest.AssertSequenceEqual(t, personNames(members), []string{"Bob"})

		ms, err := orm.Related[m2mtMembership](db, f.rock, "m2mtmembership_set").Get(ctx)
		if err != nil {
			t.Fatalf("membership get: %v", err)
		}
		djangotest.AssertEqual(t, ms.InviteReason, "He is good.")

		// Callable through_default is evaluated once per AddWith call.
		calls := 0
		reasonFn := func() any {
			calls++
			return "Reason #" + string(rune('0'+calls))
		}
		if err := orm.Many[m2mtPerson](db, f.roll, "members").AddWith(ctx, []*m2mtPerson{f.bob, f.jane},
			orm.ThroughDefaults(orm.Defaults{"invite_reason": reasonFn}),
		); err != nil {
			t.Fatalf("AddWith callable: %v", err)
		}
		djangotest.AssertEqual(t, calls, 1)
		bobMS, err := orm.Query[m2mtMembership](db).Get(ctx, "group", f.roll, "person", f.bob)
		if err != nil {
			t.Fatalf("bobMS: %v", err)
		}
		janeMS, err := orm.Query[m2mtMembership](db).Get(ctx, "group", f.roll, "person", f.jane)
		if err != nil {
			t.Fatalf("janeMS: %v", err)
		}
		djangotest.AssertEqual(t, bobMS.InviteReason, janeMS.InviteReason)

		// Required non-null through column without default succeeds with ThroughDefaults.
		if err := orm.Many[m2mtPerson](db, f.rock, "no_defaults_no_nulls").AddWith(ctx, []*m2mtPerson{f.jim},
			orm.ThroughDefaults(orm.Defaults{"no_default_no_null": int64(1)}),
		); err != nil {
			t.Fatalf("AddWith required: %v", err)
		}
		reqRow, err := orm.Query[m2mtNoDefaultsOrNulls](db).Get(ctx, "group", f.rock, "person", f.jim)
		if err != nil {
			t.Fatalf("get required row: %v", err)
		}
		djangotest.AssertEqual(t, reqRow.NoDefaultNoNull, int64(1))

		// Required non-null through column without ThroughDefaults fails with ErrIntegrity.
		err = orm.Many[m2mtPerson](db, f.roll, "no_defaults_no_nulls").Add(ctx, f.jim)
		if !errors.Is(err, orm.ErrIntegrity) {
			t.Fatalf("Add missing required field error = %v, want ErrIntegrity", err)
		}
	})
}

// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_create_on_m2m_with_intermediate_model
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_create_on_m2m_with_intermediate_model_value_required
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_create_on_m2m_with_intermediate_model_value_required_fails
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_get_or_create_on_m2m_with_intermediate_model_value_required
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_update_or_create_on_m2m_with_intermediate_model_value_required
func TestM2MThroughCreateGetOrCreateUpdateOrCreate(t *testing.T) {
	djangotest.EachDB(t, m2mtModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mtSetUp(t, db)

		annie := &m2mtPerson{Name: "Annie"}
		if err := orm.Many[m2mtPerson](db, f.rock, "members").Create(ctx, annie,
			orm.ThroughDefaults(orm.Defaults{"invite_reason": "She was just awesome."}),
		); err != nil {
			t.Fatalf("Create with through defaults: %v", err)
		}
		members, _ := orm.Many[m2mtPerson](db, f.rock, "members").All(ctx)
		djangotest.AssertSequenceEqual(t, personNames(members), []string{"Annie"})
		ms, err := orm.Related[m2mtMembership](db, f.rock, "m2mtmembership_set").Get(ctx)
		if err != nil {
			t.Fatalf("membership get: %v", err)
		}
		djangotest.AssertEqual(t, ms.InviteReason, "She was just awesome.")

		// Create with required through field.
		p1 := &m2mtPerson{Name: "Test1"}
		if err := orm.Many[m2mtPerson](db, f.rock, "no_defaults_no_nulls").Create(ctx, p1,
			orm.ThroughDefaults(orm.Defaults{"no_default_no_null": int64(1)}),
		); err != nil {
			t.Fatalf("Create required: %v", err)
		}

		// Create missing required through field fails with ErrIntegrity.
		p2 := &m2mtPerson{Name: "Test2"}
		if err := orm.Many[m2mtPerson](db, f.roll, "no_defaults_no_nulls").Create(ctx, p2); !errors.Is(err, orm.ErrIntegrity) {
			t.Fatalf("Create missing required = %v, want ErrIntegrity", err)
		}

		// GetOrCreate with ThroughDefaults.
		_, created, err := orm.Many[m2mtPerson](db, f.roll, "no_defaults_no_nulls").GetOrCreate(ctx, nil,
			"name", "TestGOC",
			orm.ThroughDefaults(orm.Defaults{"no_default_no_null": int64(2)}),
		)
		if err != nil {
			t.Fatalf("GetOrCreate: %v", err)
		}
		djangotest.AssertEqual(t, created, true)

		// UpdateOrCreate with ThroughDefaults.
		_, created, err = orm.Many[m2mtPerson](db, f.roll, "no_defaults_no_nulls").UpdateOrCreate(ctx, nil,
			"name", "TestUOC",
			orm.ThroughDefaults(orm.Defaults{"no_default_no_null": int64(3)}),
		)
		if err != nil {
			t.Fatalf("UpdateOrCreate: %v", err)
		}
		djangotest.AssertEqual(t, created, true)
	})
}

// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_remove_on_m2m_with_intermediate_model
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_remove_on_m2m_with_intermediate_model_multiple
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_set_on_m2m_with_intermediate_model_value_required
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_clear_removes_all_the_m2m_relationships
func TestM2MThroughRemoveSetClear(t *testing.T) {
	djangotest.EachDB(t, m2mtModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mtSetUp(t, db)

		// Multiple intermediate rows for same pair are both returned and both removed.
		create(t, db, &m2mtMembership{Person: fkTo[m2mtPerson](f.jim.ID), Group: fkTo[m2mtGroup](f.rock.ID), InviteReason: "1"})
		create(t, db, &m2mtMembership{Person: fkTo[m2mtPerson](f.jim.ID), Group: fkTo[m2mtGroup](f.rock.ID), InviteReason: "2"})
		members, _ := orm.Many[m2mtPerson](db, f.rock, "members").All(ctx)
		djangotest.AssertSequenceEqual(t, personNames(members), []string{"Jim", "Jim"})

		if err := orm.Many[m2mtPerson](db, f.rock, "members").Remove(ctx, f.jim); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		members, _ = orm.Many[m2mtPerson](db, f.rock, "members").All(ctx)
		djangotest.AssertEqual(t, len(members), 0)

		// Set preserves existing through row unless SetClear() is given.
		ndMgr := orm.Many[m2mtPerson](db, f.rock, "no_defaults_no_nulls")
		if err := ndMgr.Set(ctx, []*m2mtPerson{f.jim}, orm.ThroughDefaults(orm.Defaults{"no_default_no_null": int64(1)})); err != nil {
			t.Fatalf("Set 1: %v", err)
		}
		if err := ndMgr.Set(ctx, []*m2mtPerson{f.jim}, orm.ThroughDefaults(orm.Defaults{"no_default_no_null": int64(2)})); err != nil {
			t.Fatalf("Set 2 (no clear): %v", err)
		}
		row, err := orm.Query[m2mtNoDefaultsOrNulls](db).Get(ctx, "group", f.rock, "person", f.jim)
		if err != nil {
			t.Fatalf("get row: %v", err)
		}
		djangotest.AssertEqual(t, row.NoDefaultNoNull, int64(1))

		if err := ndMgr.Set(ctx, []*m2mtPerson{f.jim}, orm.SetClear(), orm.ThroughDefaults(orm.Defaults{"no_default_no_null": int64(2)})); err != nil {
			t.Fatalf("Set 2 (clear): %v", err)
		}
		row, err = orm.Query[m2mtNoDefaultsOrNulls](db).Get(ctx, "group", f.rock, "person", f.jim)
		if err != nil {
			t.Fatalf("get row after clear: %v", err)
		}
		djangotest.AssertEqual(t, row.NoDefaultNoNull, int64(2))

		// Clear removes all relationships.
		if err := ndMgr.Clear(ctx); err != nil {
			t.Fatalf("Clear: %v", err)
		}
		rem, _ := ndMgr.All(ctx)
		djangotest.AssertEqual(t, len(rem), 0)
	})
}

// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_query_model_by_attribute_name_of_related_model
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_order_by_relational_field_through_model
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_query_model_by_related_model_name
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_query_model_by_custom_related_name
// Ported from Django: tests/m2m_through/tests.py::M2mThroughTests.test_through_fields
func TestM2MThroughQueriesAndThroughFields(t *testing.T) {
	djangotest.EachDB(t, m2mtModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mtSetUp(t, db)

		create(t, db, &m2mtMembership{Person: fkTo[m2mtPerson](f.jim.ID), Group: fkTo[m2mtGroup](f.rock.ID)})
		create(t, db, &m2mtMembership{Person: fkTo[m2mtPerson](f.jane.ID), Group: fkTo[m2mtGroup](f.rock.ID)})
		create(t, db, &m2mtMembership{Person: fkTo[m2mtPerson](f.bob.ID), Group: fkTo[m2mtGroup](f.roll.ID)})

		gs, err := orm.Query[m2mtGroup](db).Filter("members__name", "Bob").All(ctx)
		if err != nil {
			t.Fatalf("Group filter members__name: %v", err)
		}
		djangotest.AssertSequenceEqual(t, groupNames(gs), []string{"Roll"})

		ps, err := orm.Query[m2mtPerson](db).Filter("m2mtgroup__name", "Rock").All(ctx)
		if err != nil {
			t.Fatalf("Person filter m2mtgroup__name: %v", err)
		}
		djangotest.AssertSequenceEqual(t, personNames(ps), []string{"Jane", "Jim"})

		// Custom related_name and order_by through model relation.
		today := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
		yesterday := today.Add(-24 * time.Hour)
		db.Now = func() time.Time { return yesterday }
		create(t, db, &m2mtCustomMembership{Person: fkTo[m2mtPerson](f.jim.ID), Group: fkTo[m2mtGroup](f.rock.ID)})
		db.Now = func() time.Time { return today }
		create(t, db, &m2mtCustomMembership{Person: fkTo[m2mtPerson](f.bob.ID), Group: fkTo[m2mtGroup](f.rock.ID)})
		db.Now = nil

		ps, err = orm.Many[m2mtPerson](db, f.rock, "custom_members").OrderBy("custom_person_related_name").All(ctx)
		if err != nil {
			t.Fatalf("order_by custom_person_related_name: %v", err)
		}
		djangotest.AssertSequenceEqual(t, personNames(ps), []string{"Jim", "Bob"})

		ps, err = orm.Query[m2mtPerson](db).Filter("custom__name", "Rock").All(ctx)
		if err != nil {
			t.Fatalf("filter custom__name: %v", err)
		}
		djangotest.AssertSequenceEqual(t, personNames(ps), []string{"Bob", "Jim"})

		// through_fields disambiguates multiple FKs to Person on Invitation.
		ev := create(t, db, &m2mtEvent{Title: "Rockwhale 2014"})
		create(t, db, &m2mtInvitation{Event: fkTo[m2mtEvent](ev.ID), Inviter: fkTo[m2mtPerson](f.bob.ID), Invitee: fkTo[m2mtPerson](f.jim.ID)})
		create(t, db, &m2mtInvitation{Event: fkTo[m2mtEvent](ev.ID), Inviter: fkTo[m2mtPerson](f.bob.ID), Invitee: fkTo[m2mtPerson](f.jane.ID)})
		invitees, err := orm.Many[m2mtPerson](db, ev, "invitees").All(ctx)
		if err != nil {
			t.Fatalf("event.invitees: %v", err)
		}
		djangotest.AssertSequenceEqual(t, personNames(invitees), []string{"Jane", "Jim"})
	})
}

// Ported from Django: tests/m2m_through/tests.py::M2mThroughReferentialTests.test_self_referential_non_symmetrical_first_side
// Ported from Django: tests/m2m_through/tests.py::M2mThroughReferentialTests.test_self_referential_non_symmetrical_second_side
// Ported from Django: tests/m2m_through/tests.py::M2mThroughReferentialTests.test_through_fields_self_referential
// Ported from Django: tests/m2m_through/tests.py::M2mThroughReferentialTests.test_add_on_symmetrical_m2m_with_intermediate_model
// Ported from Django: tests/m2m_through/tests.py::M2mThroughReferentialTests.test_set_on_symmetrical_m2m_with_intermediate_model
func TestM2MThroughSelfReferential(t *testing.T) {
	djangotest.EachDB(t, m2mtModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()

		tony := create(t, db, &m2mtPersonSelfRef{Name: "Tony"})
		chris := create(t, db, &m2mtPersonSelfRef{Name: "Chris"})
		anne := create(t, db, &m2mtPersonSelfRef{Name: "Anne"})
		kate := create(t, db, &m2mtPersonSelfRef{Name: "Kate"})

		// Non-symmetrical explicit through.
		create(t, db, &m2mtFriendship{First: fkTo[m2mtPersonSelfRef](tony.ID), Second: fkTo[m2mtPersonSelfRef](chris.ID)})
		got, err := orm.Many[m2mtPersonSelfRef](db, tony, "friends").All(ctx)
		if err != nil {
			t.Fatalf("tony.friends: %v", err)
		}
		djangotest.AssertEqual(t, len(got), 1)
		djangotest.AssertEqual(t, got[0].Name, "Chris")

		got, err = orm.Many[m2mtPersonSelfRef](db, chris, "friends").All(ctx)
		if err != nil {
			t.Fatalf("chris.friends: %v", err)
		}
		djangotest.AssertEqual(t, len(got), 0)

		// Symmetrical explicit through adds mirror entries.
		dAdd := orm.NewDate(2013, time.January, 5)
		dSet := orm.NewDate(2025, time.June, 15)
		if err := orm.Many[m2mtPersonSelfRef](db, tony, "sym_friends").AddWith(ctx, []*m2mtPersonSelfRef{anne, chris},
			orm.ThroughDefaults(orm.Defaults{"date_friended": dAdd}),
		); err != nil {
			t.Fatalf("sym_friends.AddWith: %v", err)
		}
		if err := orm.Many[m2mtPersonSelfRef](db, tony, "sym_friends").Set(ctx, []*m2mtPersonSelfRef{anne, kate},
			orm.ThroughDefaults(orm.Defaults{"date_friended": dSet}),
		); err != nil {
			t.Fatalf("sym_friends.Set: %v", err)
		}
		got, err = orm.Many[m2mtPersonSelfRef](db, tony, "sym_friends").OrderBy("name").All(ctx)
		if err != nil {
			t.Fatalf("tony.sym_friends: %v", err)
		}
		djangotest.AssertEqual(t, len(got), 2)
		djangotest.AssertEqual(t, got[0].Name, "Anne")
		djangotest.AssertEqual(t, got[1].Name, "Kate")

		anneFriends, err := orm.Many[m2mtPersonSelfRef](db, anne, "sym_friends").All(ctx)
		if err != nil {
			t.Fatalf("anne.sym_friends: %v", err)
		}
		djangotest.AssertEqual(t, len(anneFriends), 1)
		djangotest.AssertEqual(t, anneFriends[0].Name, "Tony")

		kateFriends, err := orm.Many[m2mtPersonSelfRef](db, kate, "sym_friends").All(ctx)
		if err != nil {
			t.Fatalf("kate.sym_friends: %v", err)
		}
		djangotest.AssertEqual(t, len(kateFriends), 1)
		djangotest.AssertEqual(t, kateFriends[0].Name, "Tony")

		// Anne's original date is preserved while Kate gets dSet.
		anneRel, err := orm.Query[m2mtSymmetricalFriendship](db).Get(ctx, "first", anne)
		if err != nil {
			t.Fatalf("anneRel: %v", err)
		}
		djangotest.AssertEqual(t, anneRel.DateFriended, dAdd)
		kateRel, err := orm.Query[m2mtSymmetricalFriendship](db).Get(ctx, "first", kate)
		if err != nil {
			t.Fatalf("kateRel: %v", err)
		}
		djangotest.AssertEqual(t, kateRel.DateFriended, dSet)

		// Self-referential through_fields with 3 FKs to Employee.
		john := create(t, db, &m2mtEmployee{Name: "john"})
		peter := create(t, db, &m2mtEmployee{Name: "peter"})
		mary := create(t, db, &m2mtEmployee{Name: "mary"})
		harry := create(t, db, &m2mtEmployee{Name: "harry"})
		create(t, db, &m2mtRelationship{Source: fkTo[m2mtEmployee](john.ID), Target: fkTo[m2mtEmployee](peter.ID)})
		create(t, db, &m2mtRelationship{Source: fkTo[m2mtEmployee](john.ID), Target: fkTo[m2mtEmployee](mary.ID)})
		create(t, db, &m2mtRelationship{Source: fkTo[m2mtEmployee](john.ID), Target: fkTo[m2mtEmployee](harry.ID), Another: fkTo[m2mtEmployee](peter.ID)})

		subs, err := orm.Many[m2mtEmployee](db, john, "subordinates").All(ctx)
		if err != nil {
			t.Fatalf("john.subordinates: %v", err)
		}
		subNames := make([]string, len(subs))
		for i, s := range subs {
			subNames[i] = s.Name
		}
		djangotest.AssertSequenceEqual(t, subNames, []string{"peter", "mary", "harry"})
	})
}
