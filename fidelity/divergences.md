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
`STATIC_URL="/static/"`. Regenerate the golden files from the repository root
with `python3 fidelity/oracle/render.py`.

## Skipped cases

| Case | Django | This engine |
| --- | --- | --- |
| `print_float` | `{{ 1.5 }}` prints `1.5` | prints `1.500000`; `floatformat` prints floats like Django |
| `print_none` | `{{ n }}` prints `None` | prints nothing |
| `print_list` | `['a', 'b']` (Python repr) | Go's formatting of the value; the same holds for maps |
| `lorem_words` | `{% lorem 3 w %}` prints `lorem ipsum dolor` | pongo2's tag prints `Lorem ipsum dolor` with punctuation |

## Differences without a case

### Escaping

- A filter cannot see `{% autoescape off %}`. `linebreaks`, `linebreaksbr`,
  `urlize`, `urlizetrunc`, `unordered_list` and `join` escape input that is not
  marked safe inside such a block too.
- `safe` anywhere in a filter chain turns escaping off for that output:
  `{{ v|safe|upper }}` prints unescaped, where Django escapes because `upper`
  drops the safe mark.
- `{% with x="<b>" %}{{ x }}{% endwith %}` escapes the literal. Django prints a
  string literal as written. `default`, `default_if_none`, `join`, `firstof` and
  `cycle` print their literal arguments as written, as Django does.
- An included template starts with autoescape on, also inside
  `{% autoescape off %}`.
- The safe mark is read from an unexported field of `pongo2.Value` by
  reflection. If pongo2 renames the field, every value is escaped;
  `TestIsSafe` reports that.

### Values

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
  result; malformed markup may not.
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
  `ifequal`, `length_is`, `float`, `integer`, `split`), and
  `{{ call(arg) }}` is valid.
- Error messages are pongo2's. After a rewritten tag the reported column can
  be off by a few characters, and the hidden filters `_literal_default`,
  `_literal_default_if_none`, `_literal_join`, `_dict_items`, `_dict_keys` and
  `_dict_values` can appear in a message.
- `{% load %}` accepts any library name and does nothing; every tag and filter
  is always loaded.

### Missing

- Tags: `regroup`, `resetcycle`, `querystring`, `debug`, `partialdef`,
  `partial`, and the `i18n`, `l10n`, `tz` and `cache` libraries.
- Filters: `pprint`.
- `forloop.Counter` and the other capitalized pongo2 loop names; use Django's
  lowercase names.
