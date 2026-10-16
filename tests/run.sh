#!/bin/sh
# Builds editorconfig, lays it out as rhun installs it, and runs tests/basic.txt headlessly in a
# project with .editorconfig files. usage: tests/run.sh [RHUN]
set -e
cd "$(dirname "$0")/.."
RHUN=${1:-$(command -v rhun || echo "$HOME/.local/bin/rhun")}
case "$(uname -s)-$(uname -m)" in
    Linux-x86_64) target=linux-x86_64 ;;
    Darwin-arm64) target=macos-arm64 ;;
    *) echo "no target for $(uname -s)-$(uname -m)"; exit 77 ;;
esac
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
dir=$work/exts/rhun-editor/editorconfig
mkdir -p "$dir/bin" "$work/proj/sub" "$work/config/rhun"
CGO_ENABLED=0 go build -o "$dir/bin/editorconfig-$target" .
cp extension.ini "$dir/"
printf 'root = true\n\n[*]\nindent_style = space\nindent_size = 2\ntrim_trailing_whitespace = true\n\n[*.go]\nindent_style = tab\ntab_width = 8\n\n[Makefile]\nindent_style = tab\n' > "$work/proj/.editorconfig"
printf '[*.md]\ninsert_final_newline = false\nindent_size = 3\n' > "$work/proj/sub/.editorconfig"
printf 'package main\n' > "$work/proj/main.go"
printf 'text\n' > "$work/proj/notes.txt"
printf '# notes\n' > "$work/proj/sub/readme.md"
printf 'all:\n' > "$work/proj/Makefile"
printf '[files]\nrestore_session = false\nrestore_project = false\n[updates]\ncheck = false\n[git]\nenabled = false\n[ui]\nsidebar = false\nagents_panel = false\n[editor]\ncursor_blink = false\n' > "$work/config/rhun/config"
sed "s|proj/|$work/proj/|" tests/basic.txt > "$work/basic.txt"
HOME=$work XDG_CONFIG_HOME=$work/config XDG_STATE_HOME=$work/state \
    "$RHUN" "$work/proj" --headless 1000x600 --scale 1 --extensions "$work/exts" --script "$work/basic.txt" 2>&1 \
    | sed 's/pid=[1-9][0-9]*/pid=PID/' > "$work/out.txt"
if cmp -s "$work/out.txt" tests/basic.expected; then
    echo "ok   basic"
else
    echo "FAIL basic"; diff tests/basic.expected "$work/out.txt" || true
    echo "--- the extension's log:"; cat "$work"/state/rhun/extensions/*.log 2>/dev/null | tail -20; exit 1
fi
