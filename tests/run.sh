#!/bin/sh
# Builds editorconfig, lays it out as rhun installs it, and runs each tests/NAME.txt headlessly in a
# project with .editorconfig files, comparing what rhun prints with tests/NAME.expected:
#   basic    one folder: the settings in force for files under two .editorconfig files
#   folders  protocol 1.1: two workspace folders, each its own root (workspace-folders), and a
#            .editorconfig saved with a new section applies to the open files (workspace_watch)
# usage: tests/run.sh [RHUN [NAME...]]   (UPDATE=1 rewrites the .expected files; while the SDK
# is unreleased, GOWORK=… names a go.work that uses a checkout of it)
set -e
cd "$(dirname "$0")/.."
RHUN=${1:-$(command -v rhun || echo "$HOME/.local/bin/rhun")}
[ $# -gt 0 ] && shift
names=${*:-basic folders}
case "$(uname -s)-$(uname -m)" in
    Linux-x86_64) target=linux-x86_64 ;;
    Darwin-arm64) target=macos-arm64 ;;
    *) echo "no target for $(uname -s)-$(uname -m)"; exit 77 ;;
esac
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
dir=$work/exts/rhun-editor/editorconfig
mkdir -p "$dir/bin"
CGO_ENABLED=0 go build -o "$dir/bin/editorconfig-$target" .
cp extension.ini icon.png "$dir/"
fail=0
for name in $names; do
    t=$work/$name
    mkdir -p "$t/proj/sub" "$t/config/rhun"
    printf 'root = true\n\n[*]\nindent_style = space\nindent_size = 2\ntrim_trailing_whitespace = true\n\n[*.go]\nindent_style = tab\ntab_width = 8\n\n[Makefile]\nindent_style = tab\n' > "$t/proj/.editorconfig"
    printf 'text\n' > "$t/proj/notes.txt"
    folders=$t/proj
    case $name in
    basic)
        printf '[*.md]\ninsert_final_newline = false\nindent_size = 3\n' > "$t/proj/sub/.editorconfig"
        printf 'package main\n' > "$t/proj/main.go"
        printf '# notes\n' > "$t/proj/sub/readme.md"
        printf 'all:\n' > "$t/proj/Makefile"
        ;;
    folders)
        # a second folder beside the first, without root = true: its own top is where the lookup
        # stops, so the .editorconfig above both (in $t) does not apply
        mkdir -p "$t/other/deep"
        printf '[*]\nindent_size = 5\n' > "$t/other/.editorconfig"
        printf 'text\n' > "$t/other/deep/x.txt"
        printf '[*]\ninsert_final_newline = false\n' > "$t/.editorconfig"
        folders="$t/proj $t/other"
        ;;
    esac
    printf '[files]\nrestore_session = false\nrestore_project = false\n[updates]\ncheck = false\n[git]\nenabled = false\n[ui]\nsidebar = false\nagents_panel = false\n[editor]\ncursor_blink = false\n' > "$t/config/rhun/config"
    sed "s|proj/|$t/proj/|; s|other/|$t/other/|" "tests/$name.txt" > "$t/$name.txt"
    # shellcheck disable=SC2086
    HOME=$t XDG_CONFIG_HOME=$t/config XDG_STATE_HOME=$t/state \
        "$RHUN" $folders --headless 1000x600 --scale 1 --extensions "$work/exts" --script "$t/$name.txt" 2>&1 \
        | sed "s/pid=[1-9][0-9]*/pid=PID/; s|$t/||g" > "$t/out.txt"
    [ -n "$UPDATE" ] && cp "$t/out.txt" "tests/$name.expected"
    if cmp -s "$t/out.txt" "tests/$name.expected"; then
        echo "ok   $name"
    else
        echo "FAIL $name"; diff "tests/$name.expected" "$t/out.txt" || true
        echo "--- the extension's log:"; cat "$t"/state/rhun/extensions/*.log 2>/dev/null | tail -20; fail=1
    fi
done
exit $fail
