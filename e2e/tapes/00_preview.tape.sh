Output e2e/golden/00_preview.gif

@start

# === Pull requests [2] ===
# The demo dataset opens on api-gateway #103, the most recently updated open PR.
@panel 2
@wait 400

# walk the list — the detail panel previews whatever the cursor is on
@down 2
@wait 500
@down
@wait 500

# scroll the preview without leaving the list
Type "J"
Sleep 200ms
Type "J"
Sleep 200ms
Type "K"
Sleep 200ms

# Mine tab, then back to Open
@tab_next
@wait 500
@tab_prev
@wait 500

# back to the top of the list: #103, whose pipeline failed
Type "g"
Sleep 400ms

# === Merge dialog ===
# M opens the merge form on the selected PR — strategy, close source branch and
# an optional message. Escape closes it without sending anything.
Type "M"
Sleep 900ms
@close
@wait 300

# open it in the detail panel and read further down the description
@select
@wait 600
@down 3
@wait 400

# back to the left panels
Type "h"
Sleep 300ms

# === Builds [3] ===
# The Builds panel lists the commit statuses of the selected pull request.
@panel 3
@wait 400

# drill into the pipeline run of the selected build
@select
@wait 800

# walk to the failed step and open its log
@down
@wait 400
@confirm
@wait 700

# back out: step log → pipeline → pull request
@back
@wait 500
@back
@wait 500

# === Files and the diff ===
# focus the detail panel and jump to the Files tab of #103: a modified Go file,
# its new test file and a Markdown doc
@panel 0
@files
@wait 600

# open the first file's diff — highlighted Go, with review comments marked on
# two of its lines
@confirm
@wait 800

# read down the hunk
@down 4
@wait 400

# on to the next changed file (the added test), then back out to the file list
# and out to the left panels
@next_file
@wait 800
@back
@wait 600
@back
@wait 400

# === Repos [4] ===
@panel 4
@wait 400

# skip the "All repositories" row and filter the list to one repository
@down
@select
@wait 700

# help popup
@help

@quit
