package diff

// Realistic diff fixtures. Every hunk header states counts that match the
// lines that follow it, exactly as git writes them, so that the parser's
// "the hunk has all the lines it asked for" bookkeeping is under test too.

// multiFileDiff is a three-file diff: a modified file with two hunks, an added
// file and a deleted one.
const multiFileDiff = `diff --git a/pkg/tui/app.go b/pkg/tui/app.go
index 492d9ba7970..6ddf9ae6786 100644
--- a/pkg/tui/app.go
+++ b/pkg/tui/app.go
@@ -10,6 +10,8 @@ func main() {
     ctx := context.Background()
     defer done()
-    run(ctx)
+    if err := run(ctx); err != nil {
+        return err
+    }
     log.Info("ok")
     return nil
 }
@@ -40,3 +42,4 @@ func run(ctx context.Context) error {
     select {
+    case <-ctx.Done():
     }
 }
diff --git a/deploy/staging.yml b/deploy/staging.yml
new file mode 100644
index 0000000..3f8a1c2
--- /dev/null
+++ b/deploy/staging.yml
@@ -0,0 +1,3 @@
+replicas: 2
+image: api-gateway:latest
+region: eu-west-1
diff --git a/docs/OLD.md b/docs/OLD.md
deleted file mode 100644
index 8a1b2c3..0000000
--- a/docs/OLD.md
+++ /dev/null
@@ -1,2 +0,0 @@
-# Old
-Gone.
`

// lineNumbersDiff is the smallest diff that mixes all three line kinds.
const lineNumbersDiff = `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -10,3 +10,4 @@ func main() {
 alpha
-bravo
+charlie
+delta
 echo
`

// renameDiff renames a file and changes one line inside it.
const renameDiff = `diff --git a/pkg/old/name.go b/pkg/new/name.go
similarity index 92%
rename from pkg/old/name.go
rename to pkg/new/name.go
index abc1234..def5678 100644
--- a/pkg/old/name.go
+++ b/pkg/new/name.go
@@ -1,3 +1,3 @@
-package old
+package renamed
 // Do does.
 func Do() {}
`

// copyDiff is a pure copy: no hunks at all.
const copyDiff = `diff --git a/a.txt b/b.txt
similarity index 100%
copy from a.txt
copy to b.txt
`

// modeDiff changes only the file mode.
const modeDiff = `diff --git a/scripts/run.sh b/scripts/run.sh
old mode 100644
new mode 100755
`

// binaryDiff has all three shapes of binary section: a modified one, an added
// one announced against /dev/null, and a git binary patch payload.
const binaryDiff = `diff --git a/assets/logo.png b/assets/logo.png
index abc1234..def5678 100644
Binary files a/assets/logo.png and b/assets/logo.png differ
diff --git a/assets/new.png b/assets/new.png
new file mode 100644
index 0000000..abcdef1
Binary files /dev/null and b/assets/new.png differ
diff --git a/assets/icon.ico b/assets/icon.ico
new file mode 100644
index 0000000..1234567
GIT binary patch
literal 34
mcmZ?wbhEHbWMp7uXkcJyzXcYoRfx
literal 0
HcmV?d00001
diff --git a/assets/gone.png b/assets/gone.png
deleted file mode 100644
index abcdef1..0000000
Binary files a/assets/gone.png and /dev/null differ
`

// noNewlineDiff ends both sides without a trailing newline.
const noNewlineDiff = `diff --git a/config.env b/config.env
index 1111111..2222222 100644
--- a/config.env
+++ b/config.env
@@ -1,2 +1,2 @@
 KEY=value
-TOKEN=old
\ No newline at end of file
+TOKEN=new
\ No newline at end of file
`

// quotedPathDiff is git's C-style quoted header form, used when a path needs
// escaping. The octal escapes are two UTF-8 bytes of an accented letter.
const quotedPathDiff = `diff --git "a/docs/my file.md" "b/docs/my file.md"
index 1111111..2222222 100644
--- "a/docs/my file.md"
+++ "b/docs/my file.md"
@@ -1 +1 @@
-old
+new
diff --git "a/docs/caf\303\251.md" "b/docs/caf\303\251.md"
index 3333333..4444444 100644
--- "a/docs/caf\303\251.md"
+++ "b/docs/caf\303\251.md"
@@ -1 +1 @@
-vieux
+nouveau
`

// spacedPathDiff is the unquoted-with-spaces header form git actually emits
// for a path whose only unusual character is a space.
const spacedPathDiff = `diff --git a/docs/my file.md b/docs/my file.md
index 1111111..2222222 100644
--- a/docs/my file.md
+++ b/docs/my file.md
@@ -1 +1 @@
-old
+new
`

// trailingGarbageDiff carries a git format-patch signature and loose prose
// after the last hunk.
const trailingGarbageDiff = `diff --git a/x.txt b/x.txt
index 1111111..2222222 100644
--- a/x.txt
+++ b/x.txt
@@ -1 +1 @@
-a
+b
--
2.39.5 (Apple Git-154)
some unrelated trailing text
`

// truncatedDiff is what a size-capped or timed-out response looks like: the
// last hunk promises ten lines and delivers two, then noise.
const truncatedDiff = `diff --git a/big.txt b/big.txt
index 1111111..2222222 100644
--- a/big.txt
+++ b/big.txt
@@ -1,10 +1,12 @@
 one
+two
xyzzy
`

// patchOfPatchDiff modifies a patch file, so hunk content lines start with the
// very markers a file header starts with.
const patchOfPatchDiff = `diff --git a/patches/fix.patch b/patches/fix.patch
index 1111111..2222222 100644
--- a/patches/fix.patch
+++ b/patches/fix.patch
@@ -1,3 +1,3 @@
 diff --git a/README.md b/README.md
--- old
+++ new
 tail
`

// bitbucketDiff is the live Bitbucket response quoted in the API research
// (api-rest-docs.md 6.7), continued to the two added and two removed lines
// its diffstat entry reports.
const bitbucketDiff = `diff --git a/packages/editor/conversation/src/components/Editor.tsx b/packages/editor/conversation/src/components/Editor.tsx
index 492d9ba7970..6ddf9ae6786 100644
--- a/packages/editor/conversation/src/components/Editor.tsx
+++ b/packages/editor/conversation/src/components/Editor.tsx
@@ -119,7 +119,7 @@ export default class Editor extends React.Component<Props, State> {
     const { isExpanded } = this.state;
     return (
       <div>
-        <Comment {...props} />
-        <Footer />
+        <Comment {...props} key={id} />
+        <Footer compact />
       </div>
     );
`

// garbageOnly holds no file section at all, including a metadata line that
// arrives before any file header.
const garbageOnly = `Everything below is noise.
index 1111111..2222222 100644
new file mode 100644
2.39.5 (Apple Git-154)
`

// hunkFirstDiff is the one input Parse rejects.
const hunkFirstDiff = `@@ -1 +1 @@
-a
+b
`
