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

# back to the left panels
Type "h"
Sleep 300ms

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
