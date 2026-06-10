// Command blog is a runnable Djan-Go-Go example: a small blog with a public
// post list and detail pages, reader comments submitted through a Django-style
// ModelForm, plus the staff-gated admin, wired from two model definitions.
package main

import (
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// Post is the example's primary domain model. The orm tags drive the column
// types in the same way the ORM, migrations, admin, and forms read them: Title
// is a VARCHAR(200), Body is TEXT, and Published and CreatedAt map to their
// inferred column kinds. An integer field named ID is auto-promoted to the
// primary key.
type Post struct {
	ID        int64
	Title     string `orm:"max_length=200"`
	Body      string `orm:"type=text"`
	Published bool
	CreatedAt time.Time
}

// String is Django's __str__: the admin changelist, templates, and any FK
// <select> use it as the post's human-readable label.
func (p Post) String() string { return p.Title }

// Comment is a reader comment on a Post. Post is a foreign key (stored in the
// "post_id" column); the admin renders it as a <select> of posts. CreatedAt
// carries the auto_now_add tag, so the ORM stamps it at insert time the way
// Django's models.DateTimeField(auto_now_add=True) does.
type Comment struct {
	ID        int64
	Post      orm.FK[Post]
	Name      string    `orm:"max_length=80"`
	Body      string    `orm:"type=text"`
	CreatedAt time.Time `orm:"auto_now_add"`
}

// String is Django's __str__: a short label naming the comment's author.
func (c Comment) String() string { return "Comment by " + c.Name }
