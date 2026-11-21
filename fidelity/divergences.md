# Differences from Django's template language

`render_test.go` renders every case of `cases.json` with the `templates`
package and compares the output byte for byte with `golden/<name>.txt`, which
`oracle/render.py` wrote with Django 6.0.3. A case that cannot match is named
in `skippedCases` and listed below. Differences that no case covers follow.

## Case format

A case has a `name`, a `template`, a `context` and optionally `templates`, the
named templates an `{% include %}` or `{% extends %}` refers to. Context values
are JSON with these conventions:

| JSON | Python (oracle) | Go (harness) |
| --- | --- | --- |
| number without `.` or exponent | `int` | `int` |
| other number | `float` | `float64` |
| `{"__datetime__": "2008-01-02T03:04:05"}` | aware `datetime` in UTC | `time.Time` in UTC |
| `{"__safe__": "<b>"}` | `mark_safe("<b>")` | `templates.MarkSafe("<b>")` |
| `null` | `None` | `nil` |

The oracle runs with `USE_TZ=True`, `TIME_ZONE="UTC"`, `USE_I18N=False` and
`STATIC_URL="/static/"`, plus a shared URLconf matching `fidelityRouter` in
`render_test.go`. Regenerate all golden suites from the repository root with
`go generate ./fidelity/...` (or `python3 fidelity/oracle/generate.py`), or
templates alone with `python3 fidelity/oracle/render.py`.

## Skipped cases

| Case | Django | This engine |
| --- | --- | --- |
| `print_none` | `{{ n }}` prints `None` | prints nothing |
| `print_list` | `['a', 'b']` (Python repr) | Go's formatting of the value; the same holds for maps and for `stringformat:"s"` and `make_list` of a list |
| `lorem_words` | `{% lorem 3 w %}` prints `lorem ipsum dolor` | pongo2's tag prints `Lorem ipsum dolor` with punctuation |
| `urlize_bracket_punctuation` | 6.0.3 drops the `.` of `(see www.example.com).`, a bug its main branch fixed | keeps it, as Django's main branch does |
| `csrf_token_escape` | encodes `"` as `&quot;` and `'` as `&#x27;` in `value="..."` | calls `html.EscapeString` (`&#34;` and `&#39;`, `templates/tags.go:164`) |
| `csrf_token_missing` | renders `""` when `csrf_token` is absent from context | renders `<input type="hidden" name="csrfmiddlewaretoken" value="">` (`templates/tags.go:158-165`) |

## Differences without a case

### Escaping

- A string literal is safe where Django's is: as a `{{ }}` value, a `{% with %}`
  or `{% include ... with %}` assignment and an argument of `default`,
  `default_if_none`, `join`, `firstof` and `cycle`. A literal elsewhere, for
  example the argument of a custom filter, is not marked safe.
- An included template starts with autoescape on, also inside
  `{% autoescape off %}`.
- The safe mark is read from an unexported field of `pongo2.Value` by
  reflection. If pongo2 renames the field, every value is escaped;
  `TestIsSafe` reports that.

### Values

- `{{ var.1 }}` indexes a list but does not look up the key `"1"` of a map,
  where Django tries the key first.
- `add` does not concatenate two lists.
- `{% cycle name %}` inside `{% with %}` or another block that opens a scope
  sets the variable of the cycle for that block only. Django sets it where the
  cycle defined it.
- A float prints as Django prints it; a `float32` is converted to `float64`
  first, so `float32(0.1)` prints `0.10000000149011612`.
- `1 == 1.0` is false: an `int` and a `float64` never compare equal.
- `{{ text.0 }}` on a string prints the number of the byte, not the character.
- Methods of Python types (`text.upper`, `mapping.items` outside a `{% for %}`
  tag) do not exist. `{% for k, v in mapping.items %}`, `.keys` and `.values`
  work, in sorted key order where Python keeps insertion order. `json_script`
  sorts keys for the same reason.
- A `time.Time` always has a zone, so there is no counterpart of a naive
  `datetime`.
- `upper` keeps `ß`, where Python writes `SS`.
- `slugify` folds accented Latin letters through a table. Python's NFKD
  normalization also splits characters such as `½` and `ﬁ`, which the table
  drops.
- `truncatechars_html` and `truncatewords_html` find tags with a regular
  expression where Django uses `HTMLParser`. Well-formed markup gives the same
  result; malformed markup can give a different result.
- Only English output: `date`, `timesince`, `filesizeformat`, `pluralize` and
  `yesno` are not localized.

### Syntax

- `{% if %}` conditions in Django's grammar are rewritten to get Django's
  precedence (`or` < `and` < `not` < comparisons) and the operators `not in`,
  `is` and `is not`. A condition with parentheses, `&&`, `||` or `!` is not
  valid Django, is left as written and has pongo2's meaning, where `and` and
  `or` share one precedence.
- `is` and `is not` compare by value (`==`, `!=`).
- `{% extends variable %}` is not supported; the name must be a string literal.
- pongo2's own tags and filters stay available (`set`, `macro`, `import`,
  `ifequal`, `length_is`, `float`, `integer`, `split`), and pongo2
  expressions such as `{{ call(arg) }}` and `{{ a or b }}` are valid.
- Error messages are pongo2's. After a rewritten tag the reported column can
  be off by a few characters, and the hidden filters `_out`, `_safe_literal`,
  `_literal_default`, `_literal_default_if_none`, `_literal_join`,
  `_noescape_<filter>`, `_dict_items`, `_dict_keys` and `_dict_values` can
  appear in a message.
- Some templates Django rejects render: `{{ var.__dict__ }}` (a name that
  starts with an underscore) prints nothing, `{{ va>r }}` is a comparison, and
  `{{ block.super }}` outside a child template prints nothing. `{{ 1.2.3 }}`
  fails, where Django looks up the variable.
- `{% load %}` accepts any library name and does nothing; every tag and filter
  is always loaded.

### Missing

- Tags: `regroup`, `querystring`, `debug`, `partialdef`,
  `partial`, and the `i18n`, `l10n`, `tz` and `cache` libraries.
- Filters: `pprint`.
- `forloop.Counter` and the other capitalized pongo2 loop names; use Django's
  lowercase names.
