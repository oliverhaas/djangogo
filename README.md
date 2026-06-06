# Djan-Go-Go

[![Go Reference](https://pkg.go.dev/badge/github.com/oliverhaas/djangogo.svg)](https://pkg.go.dev/github.com/oliverhaas/djangogo)
[![CI](https://github.com/oliverhaas/djangogo/actions/workflows/ci.yml/badge.svg)](https://github.com/oliverhaas/djangogo/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/oliverhaas/djangogo)](https://goreportcard.com/report/github.com/oliverhaas/djangogo)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Django's developer experience, ported to Go.**

Djan-Go-Go is a batteries-included web framework that reproduces Django's
developer experience on a pure-Go, single-binary foundation. You define a
model struct, and the ORM, migrations, admin, and forms all work from that one
definition. It is the Django analog of what Goravel is for Laravel: a faithful
DX port, built in idiomatic Go with no runtime Python.

> **Status: early proof-of-concept.** APIs are unstable and most subsystems are
> still being built. See [PLAN.md](PLAN.md) for the roadmap.

## Why

Go's web ecosystem is either lightweight routers (no ORM, no admin, no auth) or
batteries-included frameworks with their own conventions. What is missing, and
what Django developers reach for Python for, is the integrated triad of an
introspection-driven admin, pluggable apps, and an ORM with autodetecting
migrations, glued by a consistent project structure and a single `manage`-style
command.

## Install

```console
go get github.com/oliverhaas/djangogo
```

The `djangogo` CLI (project/app scaffolding, `runserver`, `makemigrations`,
`migrate`):

```console
go install github.com/oliverhaas/djangogo/cmd/djangogo@latest
```

## A taste (target API)

```go
type Article struct {
	orm.Model
	Title   string    `orm:"max_length=200"`
	Slug    string    `orm:"unique"`
	Body    string    `orm:"type=text"`
	Author  *User     `orm:"on_delete=cascade"`
	Created time.Time `orm:"auto_now_add"`
}

// Articles.Filter("author__name", "Neo").OrderBy("-created").All(ctx)
```

One declaration drives the ORM, `makemigrations`, the admin, and `ModelForm`.

## Development

```console
make            # fmt + vet + test
make test-race  # race detector
make cover      # coverage report
make lint       # golangci-lint
make run        # run the djangogo CLI
```

Requires Go 1.26+.

## License

[MIT](LICENSE)
