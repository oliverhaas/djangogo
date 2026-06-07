// Command blog is a runnable Djan-Go-Go example: a small blog with a public
// post list and detail pages plus the staff-gated admin, wired from a single
// Post model definition.
package main

import "time"

// Post is the example's only domain model. The orm tags drive the column types
// in the same way the ORM, migrations, admin, and forms read them: Title is a
// VARCHAR(200), Body is TEXT, and Published and CreatedAt map to their inferred
// column kinds. An integer field named ID is auto-promoted to the primary key.
type Post struct {
	ID        int64
	Title     string `orm:"max_length=200"`
	Body      string `orm:"type=text"`
	Published bool
	CreatedAt time.Time
}
