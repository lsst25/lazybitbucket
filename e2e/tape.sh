#!/usr/bin/env bash
# VHS tape preprocessor — expands shorthand commands into VHS tape syntax.
#
# Usage in .tape files:
#   Source e2e/tape.sh    (ignored by VHS, parsed by preprocessor)
#   @start                → Output + Set Shell/Width/Height/FontSize/Theme + launch demo
#   @down [N]             → Type "j" + Sleep (repeated N times, default 1)
#   @up [N]               → Type "k" + Sleep
#   @tab_next [N]         → Type "]" + Sleep
#   @tab_prev [N]         → Type "[" + Sleep
#   @panel N              → Type "N" + Sleep (focus panel 0-4)
#   @open                 → Type "l" + Sleep (focus the next panel; on the left side it cycles)
#   @select               → Space + Sleep (open/activate the highlighted row)
#   @back                 → Backspace + Sleep (step log → pipeline → pull request)
#   @browse               → Type "o" + Sleep (open the selection in the browser)
#   @search TEXT          → Type "/" + Sleep + Type "TEXT" + Enter + Sleep
#   @help                 → Type "?" + Sleep ... Escape + Sleep
#   @expand               → Space + Sleep (activate the highlighted block in the detail panel)
#   @close                → Escape + Sleep (close modal/overlay)
#   @quit                 → Type "q" + Sleep
#   @wait [MS]            → Sleep Nms (default 500)
#   @switch_tab           → Tab + Sleep (switch left/right panel)
#   @confirm              → Enter + Sleep (confirm modal/input)
#
# Pipe mode:   ./e2e/tape.sh e2e/tapes/foo.tape.sh | vhs -
# Env override: LAYOUT=vertical ./e2e/tape.sh file.tape.sh | vhs -
# Subcommands:
#   ./e2e/tape.sh generate e2e/tapes/foo.tape.sh     — write foo.tape + foo_vertical.tape
#   ./e2e/tape.sh generate-all                        — generate all e2e/tapes/*.tape.sh

set -euo pipefail

DEFAULT_SLEEP=200
LONG_SLEEP=400

# LAYOUT controls width in @start: horizontal (default) or vertical
LAYOUT="${LAYOUT:-horizontal}"

repeat() {
    local n="${1:-1}"
    shift
    for ((i = 0; i < n; i++)); do
        "$@"
    done
}

process_line() {
    local line="$1"

    # Strip comments on @-lines
    line="${line%%#*}"
    line="${line%"${line##*[![:space:]]}"}" # rtrim

    case "$line" in
        @start)
            local width=1200
            [[ "$LAYOUT" == "vertical" ]] && width=550
            echo 'Set Shell bash'
            echo "Set Width $width"
            echo 'Set Height 600'
            echo 'Set FontSize 14'
            echo 'Set TypingSpeed 0ms'
            echo 'Set Theme "Catppuccin Mocha"'
            echo ''
            echo 'Type "./lazybitbucket --demo"'
            echo 'Enter'
            echo 'Sleep 2s'
            ;;
        Output\ *)
            if [[ "$LAYOUT" == "vertical" ]]; then
                # insert _vertical before the extension
                local path="${line#Output }"
                local base="${path%.*}"
                local ext="${path##*.}"
                echo "Output ${base}_vertical.${ext}"
            else
                echo "$line"
            fi
            ;;
        @down*)
            local n="${line#@down}"; n="${n// /}"; n="${n:-1}"
            repeat "$n" printf 'Type "j"\nSleep %sms\n' "$DEFAULT_SLEEP"
            ;;
        @up*)
            local n="${line#@up}"; n="${n// /}"; n="${n:-1}"
            repeat "$n" printf 'Type "k"\nSleep %sms\n' "$DEFAULT_SLEEP"
            ;;
        @tab_next*)
            local n="${line#@tab_next}"; n="${n// /}"; n="${n:-1}"
            repeat "$n" printf 'Type "]"\nSleep %sms\n' "$DEFAULT_SLEEP"
            ;;
        @tab_prev*)
            local n="${line#@tab_prev}"; n="${n// /}"; n="${n:-1}"
            repeat "$n" printf 'Type "["\nSleep %sms\n' "$DEFAULT_SLEEP"
            ;;
        @panel\ *)
            local p="${line#@panel }"; p="${p// /}"
            printf 'Type "%s"\nSleep %sms\n' "$p" "$DEFAULT_SLEEP"
            ;;
        @open)
            printf 'Type "l"\nSleep %sms\n' "$LONG_SLEEP"
            ;;
        @select)
            printf 'Space\nSleep %sms\n' "$LONG_SLEEP"
            ;;
        @back)
            printf 'Backspace\nSleep %sms\n' "$LONG_SLEEP"
            ;;
        @browse)
            printf 'Type "o"\nSleep %sms\n' "$LONG_SLEEP"
            ;;
        @search\ *)
            local text="${line#@search }"
            printf 'Type "/"\nSleep %sms\n' "$DEFAULT_SLEEP"
            printf 'Set TypingSpeed 50ms\nType "%s"\nSet TypingSpeed 0ms\n' "$text"
            printf 'Sleep %sms\nEnter\nSleep %sms\n' "$LONG_SLEEP" "$DEFAULT_SLEEP"
            ;;
        @help)
            printf 'Type "?"\nSleep 500ms\nEscape\nSleep %sms\n' "$DEFAULT_SLEEP"
            ;;
        @expand)
            printf 'Space\nSleep 600ms\n'
            ;;
        @close)
            printf 'Escape\nSleep %sms\n' "$DEFAULT_SLEEP"
            ;;
        @quit)
            printf 'Sleep %sms\nType "q"\nSleep 300ms\n' "$DEFAULT_SLEEP"
            ;;
        @wait*)
            local ms="${line#@wait}"; ms="${ms// /}"; ms="${ms:-500}"
            printf 'Sleep %sms\n' "$ms"
            ;;
        @switch_tab)
            printf 'Tab\nSleep %sms\n' "$DEFAULT_SLEEP"
            ;;
        @confirm)
            printf 'Enter\nSleep %sms\n' "500"
            ;;
        @*)
            echo "# WARNING: unknown directive: $line" >&2
            ;;
        *)
            echo "$line"
            ;;
    esac
}

process_file() {
    local input="$1"
    if [[ "$input" == "-" ]]; then
        while IFS= read -r line || [[ -n "$line" ]]; do
            process_line "$line"
        done
    else
        while IFS= read -r line || [[ -n "$line" ]]; do
            process_line "$line"
        done < "$input"
    fi
}

generate_one() {
    local src="$1"
    local dir base
    dir="$(dirname "$src")"
    base="$(basename "$src" .tape.sh)"

    LAYOUT=horizontal process_file "$src" > "$dir/$base.tape"
    echo "wrote $dir/$base.tape"

    LAYOUT=vertical process_file "$src" > "$dir/${base}_vertical.tape"
    echo "wrote $dir/${base}_vertical.tape"
}

# dispatch subcommands or default pipe mode
case "${1:-}" in
    generate)
        generate_one "$2"
        ;;
    generate-all)
        for f in e2e/tapes/*.tape.sh; do
            generate_one "$f"
        done
        ;;
    *)
        # pipe mode: first arg is file path or - for stdin
        process_file "${1:--}"
        ;;
esac
