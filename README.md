# xray

`xray` is an interactive, read-only XML viewer for the terminal.

It turns XML into a compact, syntax-highlighted tree so files are easier to inspect than raw angle-bracket markup. It is inspired by [`glow`](https://github.com/charmbracelet/glow), but built specifically for XML.

## Features

- Interactive terminal UI
- Read-only by design
- Tree-first XML display
- Inline attributes and short text
- Wrapped long text without truncation
- Lightweight syntax highlighting
- Best-effort malformed XML warnings
- Fuzzy search plus XML-aware filters
- Vim-style navigation and jump list
- File input and stdin input
- SVG support as regular XML

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

Rendered conceptually as:

```text
user  @id="42"  @role="admin"  User
```

Rules:

- Repeated elements are shown separately, not grouped.
- Attributes are shown inline as `@name="value"`.
- Short text is shown inline.
- Long text is wrapped below the element.
- Whitespace in text nodes is trimmed and collapsed.
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
| `←` / `h` | Collapse / move to parent |
| `→` / `l` | Expand / move into child |
| `enter` | Expand/collapse |
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
- `catalog-large.xml` — larger generated product catalog
- `telemetry-large.xml` — larger repeated event/log-style document
- `malformed-mismatched.xml` — mismatched closing tag
- `malformed-unclosed.xml` — unclosed nested elements
- `malformed-bad-entity.xml` — undefined entity

Try one:

```sh
go run ./cmd/xray testdata/sample.xml
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
