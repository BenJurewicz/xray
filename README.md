# xray

`xray` is an interactive, read-only XML viewer for the terminal.

It turns XML into a compact, syntax-highlighted tree so files are easier to inspect than raw angle-bracket markup. It is inspired by [`glow`](https://github.com/charmbracelet/glow), but built specifically for XML.

## ⚠ Warning

This has been vibecoded because I had a need for a tool like that,
but it is surprisingly not that sloppy.

## Features

- Interactive terminal UI
- Read-only by design
- Tree-first XML display with inline attrs and text
- Long attrs/text fold independently with begin..end preview
- Terminal-width-aware soft + hard wrapping, overflow splitting
- Multiline text preserved
- Syntax highlighting: blue tags, gold attrs, green values, gray text
- Malformed XML recovery with warnings
- Fuzzy search + XML filters (tag:, attr:, text:, value:)
- Vim-style navigation with jump list
- File and stdin input
- SVG support

## Install

There are no packaged releases yet. Build from source with Go:

```sh
git clone <repo-url>
cd xray
go build ./cmd/xray
```

This creates a local `./xray` binary.

Optional install into your Go bin directory:

```sh
go install ./cmd/xray
```

## Usage

Open a file:

```sh
xray file.xml
```

Read from stdin:

```sh
cat file.xml | xray -
```

Show CLI help:

```sh
xray --help
```

## Display model

`xray` renders XML as a human-friendly tree rather than raw XML syntax.

Example XML:

```xml
<user id="42" role="admin">User</user>
```

Compact layout (all content fits on one line):

```text
user  @id="42"  @role="admin"  User
```

When an attribute value is long, all attributes move to their own lines and
each long attribute becomes individually foldable:

```text
user
   @role="admin"
   @description="A very long p..oduct description"
   Short inline text
```

When the full inline line is wider than the terminal, the same split happens
automatically. All text wraps to the terminal width with both soft
(word-boundary) and hard (column-boundary) wrapping.

Rules:

- Repeated elements are shown separately, not grouped.
- Attributes are shown inline as `@name="value"` when they all fit and are short.
- If any attribute is long (default >80 chars), all attributes move to child lines.
- If the full line exceeds terminal width, attrs and text split to child lines automatically.
- Long attribute values fold by default, showing `begin..end` preview.
- Short text is shown inline when no split is needed.
- Long text folds by default; expand to see the full wrapped content.
- Multiline text preserves newlines; each paragraph wraps independently.
- Long words that exceed the available width are hard-wrapped at column boundaries.
- Whitespace in text nodes is trimmed and collapsed (newlines preserved).
- Folded items display as `begin..end` (first 20 chars + `..` + last 20 chars).
- Malformed XML produces warnings where recovery is possible.

## Search

Press `/` in the TUI to search. Press `enter` to apply, or `esc` to cancel while typing.

Bare queries search across tag names, attribute names, attribute values, and text content:

```text
invoice
paid
```

XML-aware filters:

```text
tag:item
attr:id
attr:id=42
text:paid
value:paid
```

`value:` is an alias for `text:`.

Filters compose with spaces. Every token must match:

```text
tag:item attr:status=paid
```

After a search is active:

- `n` moves to the next match
- `N` moves to the previous match
- `esc` or `c` clears the filter
- `o` jumps back to where you were before the search
- `i` jumps forward again

## Keyboard controls

| Key | Action |
| --- | --- |
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `←` / `h` | Collapse node / fold attr or text block |
| `→` / `l` | Expand node / unfold attr or text block |
| `H` / `L` | Fold all / unfold all |
| `enter` | Toggle node / attr / text block expand |
| `d` / `u` | Half-page down / up |
| `ctrl-d` / `ctrl-u` | Half-page down / up |
| `f` / `b` | Page down / up |
| `ctrl-f` / `ctrl-b` | Page down / up |
| `e` / `y` | Scroll down / up |
| `ctrl-e` / `ctrl-y` | Scroll down / up |
| `g` / `G` | Jump to top / bottom |
| `o` / `i` | Jump backward / forward |
| `ctrl-o` / `ctrl-i` | Jump backward / forward |
| `/` | Search |
| `n` / `N` | Next / previous search match |
| `esc` / `c` | Clear active search filter |
| `?` | Toggle help |
| `q` | Quit |

## Samples

The `testdata/` directory contains files for manual testing:

- `sample.xml` — small library example
- `sample.svg` — SVG-as-XML smoke test
- `sample-long-attrs.xml` — long attribute values, folding, and multiline text
- `sample-multiline.xml` — newline preservation with paragraph breaks
- `catalog-large.xml` — larger generated product catalog
- `telemetry-large.xml` — larger repeated event/log-style document
- `malformed-mismatched.xml` — mismatched closing tag
- `malformed-unclosed.xml` — unclosed nested elements
- `malformed-bad-entity.xml` — undefined entity

Try one:

```sh
go run ./cmd/xray testdata/sample-long-attrs.xml
```

## Development

Run tests:

```sh
go test ./...
```

Build:

```sh
go build ./cmd/xray
```

The compiled `./xray` binary is ignored by git.
