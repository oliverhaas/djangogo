# Known DTL-vs-pongo2 cosmetic divergences

This catalogues template behaviours where the project's pongo2 engine produces
output that differs cosmetically from Django's DTL (the oracle). The fidelity
harness (`render_test.go`) asserts byte-equality for every faithful case and
explicitly `t.Skip`s the cases listed here, referencing this file. Divergences
are documented, not hidden: the asserted cases prove real fidelity.

## Apostrophe HTML escaping

- **Case:** `autoescape_apostrophe` (skipped).
- **Description:** When autoescaping a value containing `'`, Django emits the
  hexadecimal entity `&#x27;` whereas pongo2 emits the decimal entity `&#39;`.
  Both are valid HTML and decode to the same character, so this is purely
  cosmetic. All other escaped characters match exactly: `<` -> `&lt;`,
  `>` -> `&gt;`, `&` -> `&amp;`, `"` -> `&quot;` (verified by the asserted
  `autoescape` case, which uses `<>&"` only and renders identically).

## Non-skipped divergences avoided by case selection

These do not require a skipped case because the canonical cases are chosen to
sidestep them, but they are real and worth recording:

- **Float formatting.** A JSON number is unmarshalled into Go as `float64`.
  Printing such a value renders as `5.000000` in pongo2 versus `5.0` in Django.
  The harness therefore never prints a raw number; the numeric signal comes from
  `{{ items|length }}`, which yields an integer count formatted identically by
  both engines.
- **`forloop` counter casing.** Django exposes `{{ forloop.counter }}` while
  pongo2 exposes `{{ forloop.Counter }}` (capitalised, plus `Counter0`,
  `Revcounter`, etc.). The `for_loop` case iterates and prints each item without
  touching the loop counter, so it renders identically on both engines.
