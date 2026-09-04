package demo

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func TestNewDataset_Shape(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	testkit.AssertEqual(t, "User.UUID", data.User.UUID, uuidDemo)
	testkit.AssertEqual(t, "Workspace.Slug", data.Workspace.Slug, Workspace)
	testkit.AssertEqual(t, "len(Repositories)", len(data.Repositories), 3)
	testkit.AssertEqual(t, "len(PullRequests[api-gateway])",
		len(data.PullRequests[repoAPIGateway]), 5)
	testkit.AssertEqual(t, "len(PullRequests[web-app])", len(data.PullRequests[repoWebApp]), 3)
	testkit.AssertEqual(t, "len(PullRequests[infra])", len(data.PullRequests[repoInfra]), 2)
}

func TestNewDataset_IsDeterministic(t *testing.T) {
	t.Parallel()

	first, err := json.Marshal(NewDataset())
	if err != nil {
		t.Fatalf("marshal first dataset: %v", err)
	}
	second, err := json.Marshal(NewDataset())
	if err != nil {
		t.Fatalf("marshal second dataset: %v", err)
	}

	testkit.AssertEqual(t, "datasets are identical", string(first), string(second))
}

func TestDataset_StatusBuildNumbersResolveToPipelines(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	checked := 0

	for key, statuses := range data.Statuses {
		slug := repoSlugOfKey(t, key)
		for _, status := range statuses {
			buildNumber, ok := status.BuildNumber()
			if !ok {
				t.Fatalf("status %q on %s has no build number in url %q",
					status.Key, key, status.URL)
			}
			if !hasBuildNumber(data.Pipelines[slug], buildNumber) {
				t.Fatalf("status %q on %s points at build %d, which %s has no pipeline for",
					status.Key, key, buildNumber, slug)
			}
			checked++
		}
	}

	testkit.AssertEqual(t, "statuses checked", checked, 6)
}

func TestDataset_StatusKeysNameExistingPullRequests(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	for key := range data.Statuses {
		slug := repoSlugOfKey(t, key)
		id := idOfKey(t, key)

		found := false
		for _, pullRequest := range data.PullRequests[slug] {
			if pullRequest.ID == id {
				found = true
				testkit.AssertEqual(t, "Key()", pullRequest.Key(), key)
				break
			}
		}
		if !found {
			t.Fatalf("statuses are keyed by %q, but no such pull request exists", key)
		}
	}
}

func TestDataset_DemoUserReviewsTheExpectedPullRequests(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	reviewing := []struct {
		slug string
		id   int
	}{
		{repoAPIGateway, 102},
		{repoAPIGateway, 103},
		{repoWebApp, 40},
	}
	for _, want := range reviewing {
		pullRequest := findPullRequest(t, data, want.slug, want.id)
		testkit.AssertEqual(t, "IsReviewer(demo) on "+pullRequest.Key(),
			pullRequest.IsReviewer(uuidDemo), true)
	}

	// The demo user authors #101 and #41 and reviews neither.
	for _, authored := range []struct {
		slug string
		id   int
	}{
		{repoAPIGateway, 101},
		{repoWebApp, 41},
	} {
		pullRequest := findPullRequest(t, data, authored.slug, authored.id)
		testkit.AssertEqual(t, "IsAuthor(demo) on "+pullRequest.Key(),
			pullRequest.IsAuthor(uuidDemo), true)
		testkit.AssertEqual(t, "IsReviewer(demo) on "+pullRequest.Key(),
			pullRequest.IsReviewer(uuidDemo), false)
	}
}

func TestDataset_ReviewStatesPerPullRequest(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	approved := findPullRequest(t, data, repoAPIGateway, 101).ReviewSummary()
	testkit.AssertEqual(t, "#101 reviewers", approved.Reviewers, 2)
	testkit.AssertEqual(t, "#101 approved", approved.Approved, 1)
	testkit.AssertEqual(t, "#101 pending", approved.Pending, 1)

	draft := findPullRequest(t, data, repoAPIGateway, 102)
	testkit.AssertEqual(t, "#102 draft", draft.Draft, true)
	testkit.AssertEqual(t, "#102 approved", draft.ReviewSummary().Approved, 0)
	testkit.AssertEqual(t, "#102 reviewers", draft.ReviewSummary().Reviewers, 2)

	changes := findPullRequest(t, data, repoAPIGateway, 103).ReviewSummary()
	testkit.AssertEqual(t, "#103 changes requested", changes.ChangesRequested, 1)

	none := findPullRequest(t, data, repoWebApp, 41)
	testkit.AssertEqual(t, "#41 HasReviewData()", none.HasReviewData(), true)
	testkit.AssertEqual(t, "#41 reviewers", none.ReviewSummary().Reviewers, 0)
	testkit.AssertEqual(t, "#41 description", none.Description, "")
}

func TestDataset_ClosedPullRequestsCarryTheirClosingData(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	merged := findPullRequest(t, data, repoAPIGateway, 98)
	testkit.AssertEqual(t, "#98 State", merged.State, bitbucket.PullRequestMerged)
	if merged.MergeCommit == nil {
		t.Fatal("#98 is merged but carries no merge commit")
	}
	if merged.ClosedBy == nil {
		t.Fatal("#98 is merged but carries no closed_by")
	}

	declined := findPullRequest(t, data, repoAPIGateway, 95)
	testkit.AssertEqual(t, "#95 State", declined.State, bitbucket.PullRequestDeclined)
	testkit.AssertEqual(t, "#95 MergeCommit is nil", declined.MergeCommit == nil, true)
	if !strings.Contains(declined.Reason, "#101") {
		t.Fatalf("#95 reason = %q, want it to explain the decline", declined.Reason)
	}

	open := findPullRequest(t, data, repoInfra, 7)
	testkit.AssertEqual(t, "#7 ClosedBy is nil", open.ClosedBy == nil, true)
	testkit.AssertEqual(t, "#7 Reason", open.Reason, "")
}

func TestDataset_DescriptionsAreMarkdown(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	// #101 is the reference description: heading, list, fenced code, task list, link.
	description := findPullRequest(t, data, repoAPIGateway, 101).Description
	for _, marker := range []string{"## ", "\n- ", "```go", "- [x] ", "- [ ] ", "](https://"} {
		if !strings.Contains(description, marker) {
			t.Fatalf("#101 description is missing %q:\n%s", marker, description)
		}
	}

	// Every other non-empty description at least opens with a heading.
	for slug, pullRequests := range data.PullRequests {
		for _, pullRequest := range pullRequests {
			if pullRequest.Description == "" {
				testkit.AssertEqual(t, "only #41 has an empty description",
					slug+"#"+strconv.Itoa(pullRequest.ID), repoWebApp+"#41")
				continue
			}
			if !strings.HasPrefix(pullRequest.Description, "## ") {
				t.Fatalf("%s does not open with a heading:\n%s",
					pullRequest.Key(), pullRequest.Description)
			}
			testkit.AssertEqual(t, "Summary.Raw on "+pullRequest.Key(),
				pullRequest.Summary.Raw, pullRequest.Description)
			testkit.AssertEqual(t, "Summary.Markup on "+pullRequest.Key(),
				pullRequest.Summary.Markup, "markdown")
		}
	}
}

func TestDataset_CommitHashLengths(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	// 12 characters inside a pull request…
	for _, pullRequests := range data.PullRequests {
		for _, pullRequest := range pullRequests {
			for label, commit := range map[string]*bitbucket.CommitRef{
				"source":      pullRequest.Source.Commit,
				"destination": pullRequest.Destination.Commit,
				"merge":       pullRequest.MergeCommit,
			} {
				if commit == nil {
					continue
				}
				testkit.AssertEqual(t, pullRequest.Key()+" "+label+" hash length",
					len(commit.Hash), 12)
			}
		}
	}

	// …40 everywhere else.
	for _, statuses := range data.Statuses {
		for _, status := range statuses {
			if status.Commit == nil {
				t.Fatalf("status %q carries no commit", status.Key)
			}
			testkit.AssertEqual(t, "status commit hash length", len(status.Commit.Hash), 40)
		}
	}
	for _, pipelines := range data.Pipelines {
		for _, pipeline := range pipelines {
			if pipeline.Target.Commit == nil {
				t.Fatalf("pipeline %d carries no target commit", pipeline.BuildNumber)
			}
			testkit.AssertEqual(t, "pipeline target hash length",
				len(pipeline.Target.Commit.Hash), 40)
		}
	}
}

func TestDataset_UUIDsAreBraced(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	assertBraced(t, "user", data.User.UUID)
	assertBraced(t, "workspace", data.Workspace.UUID)
	for _, repository := range data.Repositories {
		assertBraced(t, repository.Slug, repository.UUID)
	}
	for _, pipelines := range data.Pipelines {
		for _, pipeline := range pipelines {
			assertBraced(t, "pipeline "+strconv.Itoa(pipeline.BuildNumber), pipeline.UUID)
			for _, step := range data.Steps[pipeline.UUID] {
				assertBraced(t, "step "+step.Name, step.UUID)
			}
		}
	}
}

func TestDataset_StatusURLsPointAtTheirPipeline(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	for key, statuses := range data.Statuses {
		slug := repoSlugOfKey(t, key)
		for _, status := range statuses {
			buildNumber, _ := status.BuildNumber()
			testkit.AssertEqual(t, "status url", status.URL,
				htmlBase+"/"+Workspace+"/"+slug+"/pipelines/results/"+strconv.Itoa(buildNumber))
			if status.Repository == nil {
				t.Fatalf("status %q carries no repository", status.Key)
			}
			testkit.AssertEqual(t, "repository.full_name", status.Repository.FullName,
				Workspace+"/"+slug)
			testkit.AssertEqual(t, "repository.Slug()", status.Repository.Slug(), slug)
		}
	}
}

func TestDataset_TwoStatusesOnOneCommit(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	statuses := data.Statuses[bitbucket.PullRequestKey(Workspace, repoWebApp, 40)]

	testkit.AssertEqual(t, "len(statuses)", len(statuses), 2)
	testkit.AssertEqual(t, "same commit", statuses[0].Commit.Hash, statuses[1].Commit.Hash)
	if statuses[0].Key == statuses[1].Key {
		t.Fatalf("both statuses share key %q", statuses[0].Key)
	}
	testkit.AssertEqual(t, "older state", statuses[0].State, bitbucket.StatusStopped)
	testkit.AssertEqual(t, "newer state", statuses[1].State, bitbucket.StatusSuccessful)
	if !statuses[1].UpdatedOn.After(statuses[0].UpdatedOn) {
		t.Fatalf("the SUCCESSFUL status (%s) is not newer than the STOPPED one (%s)",
			statuses[1].UpdatedOn, statuses[0].UpdatedOn)
	}

	latest := bitbucket.LatestStatuses(statuses)
	testkit.AssertEqual(t, "len(LatestStatuses)", len(latest), 2)
}

func TestDataset_RunningPipelineHasNoCompletion(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	pipeline := findPipeline(t, data, repoAPIGateway, 413)

	testkit.AssertEqual(t, "State.Name", pipeline.State.Name, stateInProgress)
	testkit.AssertEqual(t, "State.Summary()", pipeline.State.Summary(), stageRunning)
	testkit.AssertEqual(t, "CompletedOn.IsZero()", pipeline.CompletedOn.IsZero(), true)
	testkit.AssertEqual(t, "State.Result is nil", pipeline.State.Result == nil, true)

	steps := data.Steps[pipeline.UUID]
	testkit.AssertEqual(t, "len(steps)", len(steps), 4)
	testkit.AssertEqual(t, "steps[1].State.Name", steps[1].State.Name, stateInProgress)
	testkit.AssertEqual(t, "steps[2].State.Name", steps[2].State.Name, statePending)
	testkit.AssertEqual(t, "steps[3].StartedOn.IsZero()", steps[3].StartedOn.IsZero(), true)
}

func TestDataset_FailedPipelineCarriesAnErrorAndNotRunSteps(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	pipeline := findPipeline(t, data, repoAPIGateway, 414)
	steps := data.Steps[pipeline.UUID]

	testkit.AssertEqual(t, "len(steps)", len(steps), 4)

	failed := steps[1]
	if failed.State.Result == nil {
		t.Fatal("the failed step carries no result")
	}
	testkit.AssertEqual(t, "result name", failed.State.Result.Name, resultFailed)
	if failed.State.Result.Error == nil {
		t.Fatal("the failed step carries no error")
	}
	testkit.AssertEqual(t, "error key", failed.State.Result.Error.Key, "runtime.step.script.failed")
	if !strings.Contains(failed.State.Result.Error.Message, "TestRetryDeadline") {
		t.Fatalf("error message = %q, want it to name the failing test",
			failed.State.Result.Error.Message)
	}

	for _, step := range steps[2:] {
		testkit.AssertEqual(t, step.Name+" result", step.State.Result.Name, resultNotRun)
		if _, ok := data.Logs[step.UUID]; ok {
			t.Fatalf("step %q never ran but has a log", step.Name)
		}
	}
}

func TestDataset_StoppedPipelineHasATerminator(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	pipeline := findPipeline(t, data, repoInfra, 21)

	if pipeline.State.Result == nil {
		t.Fatal("the stopped run carries no result")
	}
	testkit.AssertEqual(t, "result name", pipeline.State.Result.Name, resultStopped)
	if pipeline.State.Result.Terminator == nil {
		t.Fatal("the stopped run carries no terminator")
	}
	testkit.AssertEqual(t, "terminator", pipeline.State.Result.Terminator.Name(), "Ken Thompson")
}

func TestDataset_EveryPipelineHasStepsAndEveryLogHasAStep(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	stepUUIDs := map[string]struct{}{}
	for slug, pipelines := range data.Pipelines {
		if len(pipelines) == 0 {
			t.Fatalf("%s has no pipelines", slug)
		}
		for _, pipeline := range pipelines {
			steps := data.Steps[pipeline.UUID]
			if len(steps) < 3 || len(steps) > 4 {
				t.Fatalf("build %d has %d steps, want 3 or 4", pipeline.BuildNumber, len(steps))
			}
			for _, step := range steps {
				stepUUIDs[step.UUID] = struct{}{}
			}
		}
	}

	for uuid, log := range data.Logs {
		if _, ok := stepUUIDs[uuid]; !ok {
			t.Fatalf("log %q belongs to no step", uuid)
		}
		if log == "" {
			t.Fatalf("step %q has an empty log", uuid)
		}
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func repoSlugOfKey(t *testing.T, key string) string {
	t.Helper()

	repository, _, found := strings.Cut(key, "#")
	if !found {
		t.Fatalf("status key %q has no '#'", key)
	}
	workspace, slug, found := strings.Cut(repository, "/")
	if !found {
		t.Fatalf("status key %q has no workspace", key)
	}
	testkit.AssertEqual(t, "workspace in "+key, workspace, Workspace)
	return slug
}

func idOfKey(t *testing.T, key string) int {
	t.Helper()

	_, number, found := strings.Cut(key, "#")
	if !found {
		t.Fatalf("status key %q has no '#'", key)
	}
	id, err := strconv.Atoi(number)
	if err != nil {
		t.Fatalf("status key %q has a non-numeric id: %v", key, err)
	}
	return id
}

func hasBuildNumber(pipelines []bitbucket.Pipeline, buildNumber int) bool {
	for _, pipeline := range pipelines {
		if pipeline.BuildNumber == buildNumber {
			return true
		}
	}
	return false
}

func findPullRequest(t *testing.T, data *Dataset, slug string, id int) *bitbucket.PullRequest {
	t.Helper()

	for index, pullRequest := range data.PullRequests[slug] {
		if pullRequest.ID == id {
			return &data.PullRequests[slug][index]
		}
	}
	t.Fatalf("no pull request %s#%d in the dataset", slug, id)
	return nil
}

func findPipeline(t *testing.T, data *Dataset, slug string, buildNumber int) bitbucket.Pipeline {
	t.Helper()

	for _, pipeline := range data.Pipelines[slug] {
		if pipeline.BuildNumber == buildNumber {
			return pipeline
		}
	}
	t.Fatalf("no pipeline %d in %s", buildNumber, slug)
	return bitbucket.Pipeline{}
}

func assertBraced(t *testing.T, label, uuid string) {
	t.Helper()

	if !strings.HasPrefix(uuid, "{") || !strings.HasSuffix(uuid, "}") {
		t.Errorf("%s uuid = %q, want it wrapped in braces", label, uuid)
	}
}

func TestDataset_ShiftMovesEveryTimestamp(t *testing.T) {
	t.Parallel()

	const delta = time.Hour

	before := NewDataset()
	after := NewDataset()
	after.Shift(delta)

	testkit.AssertEqual(t, "Newest()", after.Newest(), before.Newest().Add(delta))
	testkit.AssertEqual(t, "Workspace.CreatedOn",
		after.Workspace.CreatedOn, before.Workspace.CreatedOn.Add(delta))

	for index, repository := range after.Repositories {
		original := before.Repositories[index]
		testkit.AssertEqual(t, repository.Slug+".CreatedOn",
			repository.CreatedOn, original.CreatedOn.Add(delta))
		testkit.AssertEqual(t, repository.Slug+".UpdatedOn",
			repository.UpdatedOn, original.UpdatedOn.Add(delta))
	}

	for slug, pullRequests := range after.PullRequests {
		for index, pullRequest := range pullRequests {
			original := before.PullRequests[slug][index]
			label := bitbucket.PullRequestKey(Workspace, slug, pullRequest.ID)
			testkit.AssertEqual(t, label+".CreatedOn",
				pullRequest.CreatedOn, original.CreatedOn.Add(delta))
			testkit.AssertEqual(t, label+".UpdatedOn",
				pullRequest.UpdatedOn, original.UpdatedOn.Add(delta))
			for participant, shifted := range pullRequest.Participants {
				assertShifted(t, label+" participant "+shifted.User.Nickname+".ParticipatedOn",
					shifted.ParticipatedOn, original.Participants[participant].ParticipatedOn, delta)
			}
		}
	}

	for key, statuses := range after.Statuses {
		for index, status := range statuses {
			original := before.Statuses[key][index]
			testkit.AssertEqual(t, key+" "+status.Key+".CreatedOn",
				status.CreatedOn, original.CreatedOn.Add(delta))
			testkit.AssertEqual(t, key+" "+status.Key+".UpdatedOn",
				status.UpdatedOn, original.UpdatedOn.Add(delta))
		}
	}

	for slug, pipelines := range after.Pipelines {
		for index, pipeline := range pipelines {
			original := before.Pipelines[slug][index]
			label := slug + " #" + strconv.Itoa(pipeline.BuildNumber)
			testkit.AssertEqual(t, label+".CreatedOn", pipeline.CreatedOn, original.CreatedOn.Add(delta))
			assertShifted(t, label+".CompletedOn", pipeline.CompletedOn, original.CompletedOn, delta)
		}
	}

	for uuid, steps := range after.Steps {
		for index, step := range steps {
			original := before.Steps[uuid][index]
			label := uuid + " " + step.DisplayName(index)
			assertShifted(t, label+".StartedOn", step.StartedOn, original.StartedOn, delta)
			assertShifted(t, label+".CompletedOn", step.CompletedOn, original.CompletedOn, delta)
		}
	}
}

// TestDataset_ShiftPreservesOrder pins the property --demo relies on: sliding
// the world towards now must not reshuffle it.
func TestDataset_ShiftPreservesOrder(t *testing.T) {
	t.Parallel()

	const delta = 90 * day

	before := NewDataset()
	after := NewDataset()
	after.Shift(delta)

	for slug, pullRequests := range after.PullRequests {
		ids := make([]int, 0, len(pullRequests))
		want := make([]int, 0, len(pullRequests))
		for index, pullRequest := range pullRequests {
			ids = append(ids, pullRequest.ID)
			want = append(want, before.PullRequests[slug][index].ID)

			if index == 0 {
				continue
			}
			if pullRequests[index-1].UpdatedOn.Before(pullRequest.UpdatedOn) {
				t.Errorf("%s: pull request %d updated after its predecessor %d",
					slug, pullRequest.ID, pullRequests[index-1].ID)
			}
		}
		testkit.AssertSliceEqual(t, slug+" pull request order", ids, want)
	}
}

func TestDataset_ShiftByZeroChangesNothing(t *testing.T) {
	t.Parallel()

	shifted := NewDataset()
	shifted.Shift(0)

	first, err := json.Marshal(NewDataset())
	if err != nil {
		t.Fatalf("marshal dataset: %v", err)
	}
	second, err := json.Marshal(shifted)
	if err != nil {
		t.Fatalf("marshal shifted dataset: %v", err)
	}
	testkit.AssertEqual(t, "dataset", string(second), string(first))
}

// assertShifted checks a timestamp that may legitimately be absent: a zero time
// means "no such event" and must survive Shift untouched.
func assertShifted(t *testing.T, label string, got, original time.Time, delta time.Duration) {
	t.Helper()

	if original.IsZero() {
		if !got.IsZero() {
			t.Errorf("%s = %s, want it to stay absent", label, got)
		}
		return
	}
	testkit.AssertEqual(t, label, got, original.Add(delta))
}
