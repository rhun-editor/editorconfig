# editorconfig

[EditorConfig](https://editorconfig.org) for [rhun](https://rhun.app). When a file opens, the
`.editorconfig` files from its folder up to the project root (or the first with `root = true`)
are read, and the properties that match the file become rhun's per-document settings:

| EditorConfig | rhun |
| --- | --- |
| `indent_style = tab` / `space` | insert spaces off / on |
| `indent_size`, or `tab_width` when the size is `tab` or unset | tab width (1 to 16) |
| `trim_trailing_whitespace` | trim trailing whitespace on save |
| `insert_final_newline` | final newline on save |

Later files (nearer the file) and later sections win, as the specification says; glob patterns
support `*`, `**`, `?`, `[...]`, `[!...]`, `{a,b}` and `{1..3}`. The user's settings apply to
files no section matches, and again when the extension stops. `end_of_line`, `charset` and
`max_line_length` are not applied (rhun keeps a file's line endings as they are).

Command: **Show EditorConfig Settings** says what applies to the current file.

## Needs

Project: read (the `.editorconfig` files). Documents: write, because the trim-on-save and
final-newline options change what rhun writes to disk (the extension itself never edits text
or touches a file).

## Build and test

`go test ./...` covers the globs and the property mapping. `tests/run.sh [RHUN]` builds the
extension, lays it out as rhun installs it and opens files under two `.editorconfig` files in a
headless rhun, checking the settings in force for each. Releases are built by the reusable
workflow from `rhun-editor/extension-template`.
