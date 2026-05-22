# xray

`xray` is an interactive, read-only XML viewer for the terminal.

It is inspired by [`glow`](https://github.com/charmbracelet/glow), but instead of rendering Markdown, `xray` renders XML as a human-friendly tree. The goal is not to expose raw XML syntax. The goal is to make any XML file pleasant to read, regardless of how it was originally formatted.

## Goals

- Make XML readable in the terminal.
- Hide XML verbosity where it does not help comprehension.
- Preserve all meaningful structure, attributes, and text.
- Provide fast keyboard-driven navigation.
- Provide intuitive fuzzy and XML-aware search.
- Stay strictly read-only.

## Non-goals

- Editing XML.
- Reformatting files in-place.
- Acting as a validator-first tool.
- Rendering raw XML syntax as the primary view.
- Supporting huge XML files in v1.
- Supporting HTML in v1.

## v1 scope

`xray` should comfortably handle XML files around 2,000 lines.

Supported input:

```sh
xray file.xml
cat file.xml | xray -
```

Running `xray` without arguments should show help in v1. A future version may open a file picker/search rooted in the current directory.

SVG should work naturally because SVG is XML. HTML support is intentionally deferred.

## Sample files

The `testdata/` directory includes fixtures for manual testing:

- `sample.xml` — small readable library example
- `sample.svg` — SVG-as-XML smoke test
- `catalog-large.xml` — larger generated product catalog
- `telemetry-large.xml` — larger repeated event/log-style document
- `malformed-mismatched.xml` — mismatched closing tag
- `malformed-unclosed.xml` — unclosed nested elements
- `malformed-bad-entity.xml` — undefined entity

## Interface

`xray` is TUI-only for v1. There is no static print mode.

The interface is a single-pane tree viewer. Nodes and their useful details are shown directly in the tree rather than split into separate side panels.

Tree rows use lightweight syntax highlighting for element names, attributes, attribute values, and text while keeping the display compact.

### Tree display

XML elements are rendered as tree nodes.

Repeated elements are not grouped. If the source has three `<book>` elements, the viewer shows three separate `book` nodes.

Attributes are shown inline with names and values:

```text
user  @id="42" @role="admin"  User
```

Short text content is shown inline. Long text content is shown separately beneath the element.

Default short-text threshold: **80 characters**.

Whitespace in text nodes is trimmed and collapsed for readability. For example:

```xml
<title>
  Hello
</title>
```

renders as:

```text
title  Hello
```

### Long text

If an element contains text longer than the inline threshold, the element remains readable in the tree and the long text appears as expanded child content or continuation lines.

The exact visual treatment can evolve during implementation, but the principle is:

- short text: inline
- long text: visible, readable, not crammed into one row

### Malformed XML

`xray` should use best-effort parsing for malformed XML where practical.

Malformed input should not fail with a cryptic parser dump. Instead, the TUI should show visible errors or warnings while still rendering whatever structure can be recovered.

Recommended behavior:

- show a compact warning/error indicator in the interface
- include parse error entries in the tree where useful
- keep the file readable whenever recovery is possible

## Search and filtering

Search should support both broad fuzzy search and XML-specific filters.

A bare query performs fuzzy search across tag names, attribute names, attribute values, and text content.

Examples:

```text
invoice
5
User
```

Prefix filters provide XML-aware search.

Supported prefixes:

```text
tag:user
attr:id
attr:id=42
text:foo
value:5
```

`value:` is an alias for `text:`.

Filters are composable. For example:

```text
attr:id text:paid
```

matches nodes that satisfy both conditions.

When filtering, the viewer should show matching nodes plus compacted ancestors for context, rather than showing matches in isolation.

Preferred compact ancestor style:

```text
rss
└─ …/channel/item
   └─ title  Foo
```

This keeps the view compact while still explaining where the match lives.

## Keyboard controls

The TUI should use common terminal and Vim-like bindings.

Baseline controls:

| Key | Action |
| --- | --- |
| `↑` / `k` | move up |
| `↓` / `j` | move down |
| `ctrl-d` / `ctrl-u` | half-page down / up |
| `ctrl-f` / `ctrl-b` | page down / up |
| `ctrl-e` / `ctrl-y` | scroll down / up |
| `gg` / `G` | jump to top / bottom |
| `←` / `h` | collapse / move to parent |
| `→` / `l` | expand / move into child |
| `enter` | expand/collapse |
| `/` | search |
| `c` | clear search filter, visible only after a search |
| `n` | next match |
| `N` | previous match |
| `?` | help |
| `q` | quit |

## Theme

v1 should use hardcoded terminal-color-aware styling rather than a config file.

The default theme should work well with:

- dark terminals
- transparent backgrounds
- Catppuccin Frappe-like palettes

The theme should rely on terminal colors where possible instead of assuming an opaque background.

## Implementation direction

Preferred implementation language: **Go**.

Reasoning:

- portable single binaries
- strong fit for CLI/TUI tools
- good XML parsing support
- strong ecosystem around Bubble Tea and Lip Gloss
- philosophically aligned with Charm Bracelet tools like Glow

Likely libraries:

- [`bubbletea`](https://github.com/charmbracelet/bubbletea) for the TUI
- [`lipgloss`](https://github.com/charmbracelet/lipgloss) for styling
- Go's standard `encoding/xml` as the starting point for parsing

A custom tolerant/recovery layer may be needed for best-effort malformed XML handling.

## Future ideas

Not required for v1:

- file picker/search when launched without args
- configurable themes
- raw XML view toggle
- large-file streaming mode
- HTML support
- copying node paths
- exporting filtered views
