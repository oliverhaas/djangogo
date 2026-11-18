// Package auth is the port of django.contrib.auth: the user, group and
// permission models on Django's tables, password hashing in Django's formats,
// authentication backends, session-based login, access guards, the login and
// password views, the password validators and the template context processor.
//
// Settings come in through Configure; the zero Config is Django's default for
// every setting. The package builds on orm, sessions and contenttypes and does
// not import the project layer.
package auth
