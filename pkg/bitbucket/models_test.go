package bitbucket

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// timestampLayout renders a UTC time with a fixed nine-digit fraction so
// expectations are unambiguous regardless of how many digits the wire carried.
const timestampLayout = "2006-01-02T15:04:05.000000000"

// pullRequestListItemJSON is a list item from
// GET /repositories/{ws}/{slug}/pullrequests — no reviewers, no participants.
// Adapted from phaseA/api-pullrequests.md §1b.
const pullRequestListItemJSON = `{
  "comment_count": 7,
  "task_count": 0,
  "type": "pullrequest",
  "id": 101,
  "title": "Add media tags to the theme uploader",
  "description": "## Summary\n\n- adds tags\n- fixes upload validation\n",
  "state": "OPEN",
  "draft": false,
  "merge_commit": null,
  "close_source_branch": true,
  "closed_by": null,
  "author": {
    "display_name": "Yurii Tatar",
    "links": {
      "self": {"href": "https://api.bitbucket.org/2.0/users/%7Be5743438-cc31-49a0-96b7-9febd7b946f4%7D"},
      "avatar": {"href": "https://avatar.example/128"},
      "html": {"href": "https://bitbucket.org/%7Be5743438-cc31-49a0-96b7-9febd7b946f4%7D/"}
    },
    "type": "user",
    "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}",
    "account_id": "712020:e6359374-4156-4475-9f14-003f4b09cf83",
    "nickname": "Yurii Tatar"
  },
  "reason": "",
  "created_on": "2026-09-03T10:23:55.597480+00:00",
  "updated_on": "2026-09-03T14:37:36.396218+00:00",
  "destination": {
    "branch": {"name": "develop"},
    "commit": {
      "hash": "057c57734fe3",
      "links": {"self": {"href": "https://api.bitbucket.org/2.0/x"}, "html": {"href": "https://bitbucket.org/x"}},
      "type": "commit"
    },
    "repository": {
      "type": "repository",
      "full_name": "acme/api-gateway",
      "links": {
        "self": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway"},
        "html": {"href": "https://bitbucket.org/acme/api-gateway"},
        "avatar": {"href": "https://bytebucket.org/ravatar/%7B85dc8e4b%7D?ts=default"}
      },
      "name": "API Gateway",
      "uuid": "{85dc8e4b-811d-48af-b2f9-6468270ae2fb}"
    }
  },
  "source": {
    "branch": {"name": "feature/RND-14515-media-tags", "links": {}},
    "commit": {
      "hash": "d8e3c5fdfc16",
      "links": {"self": {"href": "https://api.bitbucket.org/2.0/y"}, "html": {"href": "https://bitbucket.org/y"}},
      "type": "commit"
    },
    "repository": {
      "type": "repository",
      "full_name": "acme/api-gateway",
      "links": {"self": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway"}},
      "name": "API Gateway",
      "uuid": "{85dc8e4b-811d-48af-b2f9-6468270ae2fb}"
    }
  },
  "links": {
    "self": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101"},
    "html": {"href": "https://bitbucket.org/acme/api-gateway/pull-requests/101"},
    "commits": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101/commits"},
    "approve": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101/approve"},
    "request-changes": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101/request-changes"},
    "diff": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/diff/acme/api-gateway:d8e3c5fdfc16%0D057c57734fe3?from_pullrequest_id=101&topic=true"},
    "diffstat": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/diffstat/acme/api-gateway:d8e3c5fdfc16%0D057c57734fe3?from_pullrequest_id=101&topic=true"},
    "comments": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101/comments"},
    "activity": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101/activity"},
    "merge": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101/merge"},
    "decline": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101/decline"},
    "statuses": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101/statuses"}
  },
  "summary": {
    "type": "rendered",
    "raw": "## Summary\n\n- adds tags\n- fixes upload validation\n",
    "markup": "markdown",
    "html": "<h2>Summary</h2>"
  },
  "queued": false
}`

// pullRequestDetailJSON is GET .../pullrequests/{id}: adds reviewers and
// participants. One participant has state null, one has state "approved" and a
// participated_on timestamp. Adapted from phaseA/api-pullrequests.md §2 and §3.
const pullRequestDetailJSON = `{
  "type": "pullrequest",
  "id": 9,
  "title": "Harden the upload validator",
  "description": "body",
  "state": "OPEN",
  "draft": true,
  "queued": false,
  "close_source_branch": true,
  "comment_count": 0,
  "task_count": 0,
  "merge_commit": null,
  "closed_by": null,
  "reason": "",
  "created_on": "2026-07-17T06:51:46.965721+00:00",
  "updated_on": "2026-07-20T15:24:09.475783+00:00",
  "author": {
    "display_name": "Yurii Tatar", "type": "user", "nickname": "Yurii Tatar",
    "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}",
    "account_id": "712020:e6359374-4156-4475-9f14-003f4b09cf83",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/users/a"}, "html": {"href": "https://bitbucket.org/a/"}, "avatar": {"href": "https://avatar.example/a"}}
  },
  "destination": {
    "branch": {"name": "develop"},
    "commit": {"hash": "057c57734fe3", "type": "commit", "links": {}},
    "repository": {"type": "repository", "full_name": "acme/api-gateway", "name": "API Gateway", "uuid": "{85dc8e4b}", "links": {}}
  },
  "source": {
    "branch": {"name": "bugfix/RND-13181-upload-validation"},
    "commit": {"hash": "b79b53b2aa01", "type": "commit", "links": {}},
    "repository": {"type": "repository", "full_name": "acme/api-gateway", "name": "API Gateway", "uuid": "{85dc8e4b}", "links": {}}
  },
  "summary": {"type": "rendered", "raw": "body", "markup": "markdown", "html": "<p>body</p>"},
  "reviewers": [
    {
      "display_name": "Anastasiia", "type": "user", "nickname": "Anastasiia",
      "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}", "account_id": "712020:b6b5d0ff",
      "links": {"self": {"href": "https://api.bitbucket.org/2.0/users/b"}, "html": {"href": "https://bitbucket.org/b/"}, "avatar": {"href": "https://avatar.example/b"}}
    },
    {
      "display_name": "Siamak Mirzaie", "type": "user", "nickname": "Siamak",
      "uuid": "{781a664d-34db-4bc7-9c0d-bd3716d0d1de}", "account_id": "557058:781a664d",
      "links": {"self": {"href": "https://api.bitbucket.org/2.0/users/c"}, "html": {"href": "https://bitbucket.org/c/"}, "avatar": {"href": "https://avatar.example/c"}}
    }
  ],
  "participants": [
    {
      "type": "participant",
      "user": {
        "display_name": "Anastasiia", "type": "user", "nickname": "Anastasiia",
        "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}", "account_id": "712020:b6b5d0ff",
        "links": {"self": {"href": "https://api.bitbucket.org/2.0/users/b"}, "html": {"href": "https://bitbucket.org/b/"}, "avatar": {"href": "https://avatar.example/b"}}
      },
      "role": "REVIEWER",
      "approved": true,
      "state": "approved",
      "participated_on": "2026-07-17T06:51:46.965721+00:00"
    },
    {
      "type": "participant",
      "user": {
        "display_name": "Siamak Mirzaie", "type": "user", "nickname": "Siamak",
        "uuid": "{781a664d-34db-4bc7-9c0d-bd3716d0d1de}", "account_id": "557058:781a664d",
        "links": {"self": {"href": "https://api.bitbucket.org/2.0/users/c"}, "html": {"href": "https://bitbucket.org/c/"}, "avatar": {"href": "https://avatar.example/c"}}
      },
      "role": "REVIEWER",
      "approved": false,
      "state": null,
      "participated_on": null
    }
  ],
  "links": {
    "self": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/9"},
    "html": {"href": "https://bitbucket.org/acme/api-gateway/pull-requests/9"},
    "request-changes": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/9/request-changes"},
    "statuses": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/9/statuses"}
  }
}`

// repositoryJSON is GET /repositories/{ws}/{slug}. Note links.clone is an array
// and name is not the slug. Adapted from phaseA/api-pullrequests.md §8a.
const repositoryJSON = `{
  "type": "repository",
  "uuid": "{85dc8e4b-811d-48af-b2f9-6468270ae2fb}",
  "slug": "api-gateway",
  "name": "API Gateway",
  "full_name": "acme/api-gateway",
  "description": "",
  "scm": "git",
  "language": "typescript",
  "is_private": true,
  "size": 777020962,
  "created_on": "2017-10-16T08:32:04.844941+00:00",
  "updated_on": "2026-09-03T17:02:02.828459+00:00",
  "mainbranch": {"name": "develop", "type": "branch"},
  "project": {
    "type": "project", "key": "PLAT", "name": "Platform", "uuid": "{1c2}",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/p"}, "html": {"href": "https://bitbucket.org/p"}, "avatar": {"href": "https://avatar.example/p"}}
  },
  "workspace": {
    "type": "workspace", "uuid": "{1211431e-b8a4-44b1-a3c1-5e8721b99a27}", "slug": "acme", "name": "Acme Corp",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/w"}, "html": {"href": "https://bitbucket.org/acme/"}, "avatar": {"href": "https://avatar.example/w"}}
  },
  "owner": {
    "display_name": "Acme", "type": "team", "uuid": "{1211431e-b8a4-44b1-a3c1-5e8721b99a27}",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/workspaces/acme"}, "html": {"href": "https://bitbucket.org/acme/"}, "avatar": {"href": "https://avatar.example/o"}}
  },
  "links": {
    "self": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway"},
    "html": {"href": "https://bitbucket.org/acme/api-gateway"},
    "avatar": {"href": "https://bytebucket.org/ravatar/%7B85dc8e4b%7D"},
    "pullrequests": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests"},
    "commits": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/commits"},
    "branches": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/refs/branches"},
    "clone": [
      {"name": "https", "href": "https://demo@bitbucket.org/acme/api-gateway.git"},
      {"name": "ssh", "href": "git@bitbucket.org:acme/api-gateway.git"}
    ]
  }
}`

// commitStatusJSON is one item of GET .../pullrequests/{id}/statuses.
// Adapted from phaseA/api-builds-pipelines.md §2.5.
const commitStatusJSON = `{
  "key": "prs:**:develop",
  "type": "build",
  "state": "STOPPED",
  "name": "Pipeline - pullrequests: **",
  "refname": "bugfix/RND-13181-upload-validation",
  "description": "",
  "url": "https://bitbucket.org/acme/api-gateway/pipelines/results/10471",
  "commit": {
    "hash": "861daec308a82db4781754d2feea3ee0e6d2c7e2",
    "type": "commit",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/c"}, "html": {"href": "https://bitbucket.org/c"}}
  },
  "repository": {
    "type": "repository", "full_name": "acme/api-gateway", "name": "API Gateway", "uuid": "{85dc8e4b}",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/r"}, "html": {"href": "https://bitbucket.org/r"}, "avatar": {"href": "https://bytebucket.org/ravatar/x"}}
  },
  "created_on": "2026-09-03T14:27:01.326890+00:00",
  "updated_on": "2026-09-03T14:28:44.231234+00:00",
  "links": {
    "self": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/commit/861daec3/statuses/build/prs:**:develop"},
    "commit": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/commit/861daec3"}
  }
}`

// pipelinePullRequestTargetJSON is a PR-target run that was STOPPED by a user.
// Adapted from phaseA/api-builds-pipelines.md §3.5 and §3.6.
const pipelinePullRequestTargetJSON = `{
  "type": "pipeline",
  "uuid": "{210f1f44-67d9-4b63-b755-7f46d0af56d4}",
  "pipeline_request_uuid": "{210f1f44-67d9-4b63-b755-7f46d0af56d4}",
  "build_number": 10471,
  "run_number": 1,
  "state": {
    "name": "COMPLETED",
    "type": "pipeline_state_completed",
    "result": {
      "name": "STOPPED",
      "type": "pipeline_state_completed_stopped",
      "terminator": {
        "display_name": "Anastasiia", "type": "user", "nickname": "Anastasiia",
        "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}", "account_id": "712020:b6b5d0ff",
        "links": {"self": {"href": "https://api.bitbucket.org/2.0/users/b"}, "html": {"href": "https://bitbucket.org/b/"}, "avatar": {"href": "https://avatar.example/b"}}
      }
    }
  },
  "creator": {"type": "user", "uuid": "{42be8e4e-1f37-474c-8550-9da23cb86f19}"},
  "repository": {
    "type": "repository", "full_name": "acme/api-gateway", "name": "API Gateway", "uuid": "{85dc8e4b}",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/r"}, "html": {"href": "https://bitbucket.org/r"}, "avatar": {"href": "https://bytebucket.org/ravatar/x"}}
  },
  "target": {
    "type": "pipeline_pullrequest_target",
    "selector": {"type": "pull-requests", "pattern": "**"},
    "commit": {"hash": "861daec308a82db4781754d2feea3ee0e6d2c7e2", "type": "commit", "links": {}},
    "source": "bugfix/RND-13181-upload-validation",
    "destination": "develop",
    "destination_commit": {"hash": "87d7f78f5461", "type": "commit", "links": {}},
    "pullrequest": {
      "type": "pullrequest", "id": 9, "title": "Harden the upload validator", "draft": false, "queued": false,
      "links": {
        "self": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/9"},
        "html": {"href": "https://bitbucket.org/acme/api-gateway/pull-requests/9"}
      }
    }
  },
  "trigger": {"name": "PUSH", "type": "pipeline_trigger_push"},
  "created_on": "2026-09-03T16:23:58.0060857Z",
  "completed_on": "2026-09-03T16:34:20.692470166Z",
  "run_creation_date": "2026-09-03T16:23:58.012597823Z",
  "duration_in_seconds": 586,
  "build_seconds_used": 1113,
  "first_successful": false,
  "expired": false,
  "has_variables": false,
  "labels": {},
  "links": {
    "self": {"href": "https://api.bitbucket.org/2.0/repositories/%7Bws%7D/%7Brepo%7D/pipelines/%7B210f1f44%7D"},
    "steps": {"href": "https://api.bitbucket.org/2.0/repositories/%7Bws%7D/%7Brepo%7D/pipelines/%7B210f1f44%7D/steps"}
  }
}`

// pipelineStepFailedJSON is a modern step that FAILED with an error payload.
// Adapted from phaseA/api-builds-pipelines.md §5.3.
const pipelineStepFailedJSON = `{
  "type": "pipeline_step",
  "uuid": "{9520b082-5efb-45c5-8538-9aeab8f74301}",
  "name": "Cancel Previous Pipelines",
  "trigger": {"type": "pipeline_step_trigger_automatic"},
  "state": {
    "name": "COMPLETED",
    "type": "pipeline_step_state_completed",
    "result": {
      "name": "FAILED",
      "type": "pipeline_step_state_completed_failed",
      "error": {
        "type": "pipeline_step_error",
        "key": "agent.git.git-branch-not-found",
        "message": "The branch was not found",
        "arguments": {}
      }
    }
  },
  "started_on": "2026-07-02T08:49:39.304503031Z",
  "completed_on": "2026-07-02T08:50:14.41318653Z",
  "duration_in_seconds": 35,
  "build_seconds_used": 35,
  "run_number": 1,
  "maxTime": 1,
  "image": {"name": "alpine:latest"},
  "pipeline": {"type": "pipeline", "uuid": "{210f1f44-67d9-4b63-b755-7f46d0af56d4}"},
  "setup_commands": [{"commandType": "user", "name": "umask 000", "command": "umask 000"}],
  "script_commands": [{"commandType": "user", "name": "sh scripts/cancel.sh", "command": "sh scripts/cancel.sh"}],
  "teardown_commands": [
    {"commandType": "system", "name": "Upload Metrics", "command": "echo \"Uploading metrics\"", "action": "UPLOAD_METRICS"}
  ]
}`

// pipelineStepLegacyJSON is a 2017-era step: no name, no teardown_commands, no
// links. Adapted from phaseA/api-builds-pipelines.md §5.4.
const pipelineStepLegacyJSON = `{
  "type": "pipeline_step",
  "uuid": "{8fc2d263-a4a6-4e15-b898-2c0f827f881c}",
  "trigger": {"type": "pipeline_step_trigger_automatic"},
  "state": {
    "name": "COMPLETED",
    "type": "pipeline_step_state_completed",
    "result": {"name": "STOPPED", "type": "pipeline_step_state_completed_stopped"}
  },
  "started_on": "2017-11-02T17:10:22.427Z",
  "completed_on": "2017-11-02T17:10:36.585Z",
  "duration_in_seconds": 14,
  "build_seconds_used": 14,
  "run_number": 1,
  "maxTime": 120,
  "image": {"name": "debian:8.6"},
  "pipeline": {"type": "pipeline", "uuid": "{b6484b8a-a198-4d94-96e8-a0256068ac13}"},
  "setup_commands": [{"commandType": "user", "name": "Setting up volume permissions", "command": "umask 000"}],
  "script_commands": [{"commandType": "user", "name": "make test", "command": "make test"}]
}`

func decodeJSON[T any](t *testing.T, data string) T {
	t.Helper()

	var out T
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		t.Fatalf("unmarshal into %T: %v", out, err)
	}
	return out
}

func assertTime(t *testing.T, label string, got time.Time, want string) {
	t.Helper()

	testkit.AssertEqual(t, label, got.UTC().Format(timestampLayout), want)
}

func TestPullRequest_UnmarshalListItem(t *testing.T) {
	t.Parallel()

	pr := decodeJSON[PullRequest](t, pullRequestListItemJSON)

	testkit.AssertEqual(t, "pr.ID", pr.ID, 101)
	testkit.AssertEqual(t, "pr.Type", pr.Type, "pullrequest")
	testkit.AssertEqual(t, "pr.Title", pr.Title, "Add media tags to the theme uploader")
	testkit.AssertEqual(t, "pr.State", pr.State, PullRequestOpen)
	testkit.AssertEqual(t, "pr.Draft", pr.Draft, false)
	testkit.AssertEqual(t, "pr.Queued", pr.Queued, false)
	testkit.AssertEqual(t, "pr.CloseSourceBranch", pr.CloseSourceBranch, true)
	testkit.AssertEqual(t, "pr.CommentCount", pr.CommentCount, 7)
	testkit.AssertEqual(t, "pr.TaskCount", pr.TaskCount, 0)
	testkit.AssertEqual(t, "pr.Reason", pr.Reason, "")
	testkit.AssertEqual(t, "pr.Author.Name()", pr.Author.Name(), "Yurii Tatar")
	testkit.AssertEqual(t, "pr.Author.UUID", pr.Author.UUID, testAuthorUUID)
	testkit.AssertEqual(t, "pr.Summary.Markup", pr.Summary.Markup, "markdown")
	testkit.AssertEqual(t, "pr.Description", pr.Description, pr.Summary.Raw)

	if pr.ClosedBy != nil {
		t.Errorf("pr.ClosedBy = %+v, want nil while OPEN", pr.ClosedBy)
	}
	if pr.MergeCommit != nil {
		t.Errorf("pr.MergeCommit = %+v, want nil while OPEN", pr.MergeCommit)
	}

	assertTime(t, "pr.CreatedOn", pr.CreatedOn, "2026-09-03T10:23:55.597480000")
	assertTime(t, "pr.UpdatedOn", pr.UpdatedOn, "2026-09-03T14:37:36.396218000")

	testkit.AssertEqual(t, "pr.Workspace()", pr.Workspace(), testWorkspace)
	testkit.AssertEqual(t, "pr.RepoSlug()", pr.RepoSlug(), testRepoSlug)
	testkit.AssertEqual(t, "pr.RepoFullName()", pr.RepoFullName(), testRepoFull)
	testkit.AssertEqual(t, "pr.Key()", pr.Key(), "acme/api-gateway#101")
	testkit.AssertEqual(t, "pr.SourceBranch()", pr.SourceBranch(), "feature/RND-14515-media-tags")
	testkit.AssertEqual(t, "pr.DestinationBranch()", pr.DestinationBranch(), "develop")
	testkit.AssertEqual(t, "pr.HTMLURL()", pr.HTMLURL(),
		"https://bitbucket.org/acme/api-gateway/pull-requests/101")

	if pr.Source.Commit == nil {
		t.Fatal("pr.Source.Commit = nil, want the abbreviated head commit")
	}
	testkit.AssertEqual(t, "pr.Source.Commit.Hash", pr.Source.Commit.Hash, "d8e3c5fdfc16")

	// A list item without a fields= projection carries no review data.
	testkit.AssertEqual(t, "pr.HasReviewData()", pr.HasReviewData(), false)
	testkit.AssertEqual(t, "len(pr.Reviewers)", len(pr.Reviewers), 0)
	testkit.AssertEqual(t, "pr.ReviewSummary()", pr.ReviewSummary(), ReviewSummary{
		Reviewers: 0, Approved: 0, ChangesRequested: 0, Pending: 0,
	})

	testkit.AssertEqual(t, "pr.Links.RequestChanges.Href", pr.Links.RequestChanges.Href,
		"https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101/request-changes")
	testkit.AssertEqual(t, "pr.Links.Statuses.Href", pr.Links.Statuses.Href,
		"https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101/statuses")
}

func TestPullRequest_UnmarshalDetailWithParticipants(t *testing.T) {
	t.Parallel()

	pr := decodeJSON[PullRequest](t, pullRequestDetailJSON)

	testkit.AssertEqual(t, "pr.ID", pr.ID, 9)
	testkit.AssertEqual(t, "pr.Draft", pr.Draft, true)
	testkit.AssertEqual(t, "pr.HasReviewData()", pr.HasReviewData(), true)
	testkit.AssertEqual(t, "len(pr.Reviewers)", len(pr.Reviewers), 2)

	if len(pr.Participants) != 2 {
		t.Fatalf("len(pr.Participants) = %d, want 2", len(pr.Participants))
	}

	approver := pr.Participants[0]
	testkit.AssertEqual(t, "participant[0].Role", approver.Role, RoleReviewer)
	testkit.AssertEqual(t, "participant[0].Approved", approver.Approved, true)
	testkit.AssertEqual(t, "participant[0].State", approver.State, ParticipantApproved)
	assertTime(t, "participant[0].ParticipatedOn", approver.ParticipatedOn,
		"2026-07-17T06:51:46.965721000")

	// state: null and participated_on: null decode to the zero values.
	pending := pr.Participants[1]
	testkit.AssertEqual(t, "participant[1].State", pending.State, ParticipantState(""))
	testkit.AssertEqual(t, "participant[1].Approved", pending.Approved, false)
	if !pending.ParticipatedOn.IsZero() {
		t.Errorf("participant[1].ParticipatedOn = %v, want zero", pending.ParticipatedOn)
	}

	testkit.AssertEqual(t, "pr.ReviewSummary()", pr.ReviewSummary(), ReviewSummary{
		Reviewers: 2, Approved: 1, ChangesRequested: 0, Pending: 1,
	})

	testkit.AssertEqual(t, "pr.IsAuthor(author)", pr.IsAuthor(testAuthorUUID), true)
	testkit.AssertEqual(t, "pr.IsAuthor(reviewer)", pr.IsAuthor(testReviewerUUID), false)
	testkit.AssertEqual(t, `pr.IsAuthor("")`, pr.IsAuthor(""), false)
	testkit.AssertEqual(t, "pr.IsReviewer(reviewer)", pr.IsReviewer(testReviewerUUID), true)
	testkit.AssertEqual(t, "pr.IsReviewer(author)", pr.IsReviewer(testAuthorUUID), false)
	testkit.AssertEqual(t, `pr.IsReviewer("")`, pr.IsReviewer(""), false)
	testkit.AssertEqual(t, "pr.Key()", pr.Key(), "acme/api-gateway#9")
}

func TestPullRequest_HTMLURLFallbackWithoutLinks(t *testing.T) {
	t.Parallel()

	pr := PullRequest{ID: 42}
	pr.Destination.Repository.FullName = testRepoFull

	testkit.AssertEqual(t, "pr.HTMLURL()", pr.HTMLURL(),
		"https://bitbucket.org/acme/api-gateway/pull-requests/42")
	testkit.AssertEqual(t, "PullRequestKey", PullRequestKey(testWorkspace, testRepoSlug, 42),
		"acme/api-gateway#42")
}

func TestPullRequest_RepoFullNameFallsBackToName(t *testing.T) {
	t.Parallel()

	pr := PullRequest{ID: 1}
	pr.Destination.Repository.Name = "API Gateway"

	testkit.AssertEqual(t, "pr.RepoFullName()", pr.RepoFullName(), "API Gateway")
	testkit.AssertEqual(t, "pr.Workspace()", pr.Workspace(), "")
	testkit.AssertEqual(t, "pr.RepoSlug()", pr.RepoSlug(), "API Gateway")
}

func TestPullRequest_ReviewSummary(t *testing.T) {
	t.Parallel()

	reviewer := Account{UUID: testReviewerUUID, DisplayName: "Anastasiia"}
	other := Account{UUID: testOtherUUID, DisplayName: "Siamak Mirzaie"}
	author := Account{UUID: testAuthorUUID, DisplayName: "Yurii Tatar"}

	tests := []struct {
		name string
		pr   PullRequest
		want ReviewSummary
	}{
		{
			name: "no review data",
			pr:   PullRequest{},
			want: ReviewSummary{Reviewers: 0, Approved: 0, ChangesRequested: 0, Pending: 0},
		},
		{
			name: "reviewers loaded but nobody acted",
			pr: PullRequest{
				Reviewers:    []Account{reviewer, other},
				Participants: []Participant{},
			},
			want: ReviewSummary{Reviewers: 2, Approved: 0, ChangesRequested: 0, Pending: 2},
		},
		{
			name: "approved flag without a state still counts",
			pr: PullRequest{
				Reviewers: []Account{reviewer},
				Participants: []Participant{
					{User: reviewer, Role: RoleReviewer, Approved: true},
				},
			},
			want: ReviewSummary{Reviewers: 1, Approved: 1, ChangesRequested: 0, Pending: 0},
		},
		{
			name: "changes requested",
			pr: PullRequest{
				Reviewers: []Account{reviewer, other},
				Participants: []Participant{
					{User: reviewer, Role: RoleReviewer, State: ParticipantChangesRequested},
					{User: other, Role: RoleReviewer},
				},
			},
			want: ReviewSummary{Reviewers: 2, Approved: 0, ChangesRequested: 1, Pending: 1},
		},
		{
			name: "participant role who approved counts as a reviewer",
			pr: PullRequest{
				Reviewers: []Account{reviewer},
				Participants: []Participant{
					{User: reviewer, Role: RoleReviewer, Approved: true, State: ParticipantApproved},
					{User: author, Role: RoleParticipant, Approved: true, State: ParticipantApproved},
				},
			},
			want: ReviewSummary{Reviewers: 2, Approved: 2, ChangesRequested: 0, Pending: 0},
		},
		{
			name: "commenting participant is neither reviewer nor approver",
			pr: PullRequest{
				Reviewers: []Account{reviewer},
				Participants: []Participant{
					{User: author, Role: RoleParticipant},
				},
			},
			want: ReviewSummary{Reviewers: 1, Approved: 0, ChangesRequested: 0, Pending: 1},
		},
		{
			name: "duplicate reviewer and participant entries collapse by uuid",
			pr: PullRequest{
				Reviewers: []Account{reviewer, reviewer},
				Participants: []Participant{
					{User: reviewer, Role: RoleReviewer, Approved: true, State: ParticipantApproved},
				},
			},
			want: ReviewSummary{Reviewers: 1, Approved: 1, ChangesRequested: 0, Pending: 0},
		},
		{
			name: "approvals beyond the requested reviewers never go negative",
			pr: PullRequest{
				Participants: []Participant{
					{User: reviewer, Role: RoleParticipant, Approved: true},
					{User: other, Role: RoleParticipant, State: ParticipantApproved},
				},
			},
			want: ReviewSummary{Reviewers: 2, Approved: 2, ChangesRequested: 0, Pending: 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "ReviewSummary()", tc.pr.ReviewSummary(), tc.want)
		})
	}
}

func TestAccount_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account Account
		want    string
	}{
		{
			name:    "display name wins",
			account: Account{DisplayName: "Yurii Tatar", Nickname: "yuri", UUID: testAuthorUUID},
			want:    "Yurii Tatar",
		},
		{
			name:    "nickname when display name is empty",
			account: Account{Nickname: "yuri", UUID: testAuthorUUID},
			want:    "yuri",
		},
		{
			name:    "uuid for a degenerate stub account",
			account: Account{Type: "user", UUID: testAuthorUUID},
			want:    testAuthorUUID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "Name()", tc.account.Name(), tc.want)
		})
	}
}

func TestRepositoryRef_WorkspaceAndSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		ref           RepositoryRef
		wantWorkspace string
		wantSlug      string
	}{
		{
			name:          "slug comes from full_name, not name",
			ref:           RepositoryRef{Name: "Sales ERP", FullName: "acme/sales-erp"},
			wantWorkspace: testWorkspace,
			wantSlug:      "sales-erp",
		},
		{
			name:          "no slash falls back to name",
			ref:           RepositoryRef{Name: "API Gateway"},
			wantWorkspace: "",
			wantSlug:      "API Gateway",
		},
		{
			name:          "trailing slash falls back to name",
			ref:           RepositoryRef{Name: "API Gateway", FullName: "acme/"},
			wantWorkspace: testWorkspace,
			wantSlug:      "API Gateway",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "Workspace()", tc.ref.Workspace(), tc.wantWorkspace)
			testkit.AssertEqual(t, "Slug()", tc.ref.Slug(), tc.wantSlug)
		})
	}
}

func TestRepository_Unmarshal(t *testing.T) {
	t.Parallel()

	repo := decodeJSON[Repository](t, repositoryJSON)

	testkit.AssertEqual(t, "repo.Slug", repo.Slug, testRepoSlug)
	testkit.AssertEqual(t, "repo.Name", repo.Name, "API Gateway")
	testkit.AssertEqual(t, "repo.FullName", repo.FullName, testRepoFull)
	testkit.AssertEqual(t, "repo.SCM", repo.SCM, "git")
	testkit.AssertEqual(t, "repo.Language", repo.Language, "typescript")
	testkit.AssertEqual(t, "repo.IsPrivate", repo.IsPrivate, true)
	testkit.AssertEqual(t, "repo.Size", repo.Size, int64(777020962))
	testkit.AssertEqual(t, "repo.MainBranchName()", repo.MainBranchName(), "develop")
	testkit.AssertEqual(t, "repo.WorkspaceSlug()", repo.WorkspaceSlug(), testWorkspace)
	testkit.AssertEqual(t, "repo.HTMLURL()", repo.HTMLURL(), "https://bitbucket.org/acme/api-gateway")
	assertTime(t, "repo.CreatedOn", repo.CreatedOn, "2017-10-16T08:32:04.844941000")
	assertTime(t, "repo.UpdatedOn", repo.UpdatedOn, "2026-09-03T17:02:02.828459000")

	if repo.Project == nil {
		t.Fatal("repo.Project = nil, want the PLAT project")
	}
	testkit.AssertEqual(t, "repo.Project.Key", repo.Project.Key, "PLAT")

	if repo.Owner == nil {
		t.Fatal("repo.Owner = nil, want the workspace team")
	}
	testkit.AssertEqual(t, "repo.Owner.Type", repo.Owner.Type, "team")

	// links.clone is an array of {name, href}, unlike every other links member.
	if len(repo.Links.Clone) != 2 {
		t.Fatalf("len(repo.Links.Clone) = %d, want 2", len(repo.Links.Clone))
	}
	testkit.AssertEqual(t, "clone[1].Name", repo.Links.Clone[1].Name, "ssh")
	testkit.AssertEqual(t, "clone[1].Href", repo.Links.Clone[1].Href,
		"git@bitbucket.org:acme/api-gateway.git")
	testkit.AssertEqual(t, "repo.Links.PullRequests.Href", repo.Links.PullRequests.Href,
		"https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests")
	testkit.AssertEqual(t, "repo.Links.Branches.Href", repo.Links.Branches.Href,
		"https://api.bitbucket.org/2.0/repositories/acme/api-gateway/refs/branches")
}

func TestRepository_FallbacksWithoutEmbeddedObjects(t *testing.T) {
	t.Parallel()

	repo := Repository{FullName: testRepoFull}

	testkit.AssertEqual(t, "repo.HTMLURL()", repo.HTMLURL(), "https://bitbucket.org/acme/api-gateway")
	testkit.AssertEqual(t, "repo.WorkspaceSlug()", repo.WorkspaceSlug(), testWorkspace)
	testkit.AssertEqual(t, "repo.MainBranchName()", repo.MainBranchName(), "")
	testkit.AssertEqual(t, "bare repo WorkspaceSlug()", Repository{}.WorkspaceSlug(), "")
}

func TestCommitStatus_Unmarshal(t *testing.T) {
	t.Parallel()

	status := decodeJSON[CommitStatus](t, commitStatusJSON)

	testkit.AssertEqual(t, "status.Type", status.Type, "build")
	testkit.AssertEqual(t, "status.Key", status.Key, "prs:**:develop")
	testkit.AssertEqual(t, "status.State", status.State, StatusStopped)
	testkit.AssertEqual(t, "status.Name", status.Name, "Pipeline - pullrequests: **")
	testkit.AssertEqual(t, "status.Description", status.Description, "")
	testkit.AssertEqual(t, "status.RefName", status.RefName, "bugfix/RND-13181-upload-validation")
	assertTime(t, "status.CreatedOn", status.CreatedOn, "2026-09-03T14:27:01.326890000")
	assertTime(t, "status.UpdatedOn", status.UpdatedOn, "2026-09-03T14:28:44.231234000")

	if status.Commit == nil {
		t.Fatal("status.Commit = nil, want the full 40-char commit")
	}
	testkit.AssertEqual(t, "len(status.Commit.Hash)", len(status.Commit.Hash), 40)

	if status.Repository == nil {
		t.Fatal("status.Repository = nil, want the abbreviated repository")
	}
	testkit.AssertEqual(t, "status.Repository.Slug()", status.Repository.Slug(), testRepoSlug)

	build, ok := status.BuildNumber()
	testkit.AssertEqual(t, "ok", ok, true)
	testkit.AssertEqual(t, "build", build, 10471)
}

func TestCommitStatus_BuildNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want int
		ok   bool
	}{
		{
			name: "pipelines result url",
			url:  "https://bitbucket.org/acme/api-gateway/pipelines/results/10474",
			want: 10474,
			ok:   true,
		},
		{
			name: "trailing slash",
			url:  "https://bitbucket.org/acme/api-gateway/pipelines/results/7/",
			want: 7,
			ok:   true,
		},
		{
			name: "third-party ci url",
			url:  "https://ci.example.com/builds/9912",
			want: 0,
			ok:   false,
		},
		{
			name: "step suffix is not an integer",
			url:  "https://bitbucket.org/acme/api-gateway/pipelines/results/10474/steps/abc",
			want: 0,
			ok:   false,
		},
		{
			name: "no build number at all",
			url:  "https://bitbucket.org/acme/api-gateway/pipelines/results/",
			want: 0,
			ok:   false,
		},
		{
			name: "empty url",
			url:  "",
			want: 0,
			ok:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := CommitStatus{URL: tc.url}.BuildNumber()
			testkit.AssertEqual(t, "ok", ok, tc.ok)
			testkit.AssertEqual(t, "build number", got, tc.want)
		})
	}
}

func TestPipeline_UnmarshalPullRequestTarget(t *testing.T) {
	t.Parallel()

	pipeline := decodeJSON[Pipeline](t, pipelinePullRequestTargetJSON)

	testkit.AssertEqual(t, "pipeline.UUID", pipeline.UUID, "{210f1f44-67d9-4b63-b755-7f46d0af56d4}")
	testkit.AssertEqual(t, "pipeline.BuildNumber", pipeline.BuildNumber, 10471)
	testkit.AssertEqual(t, "pipeline.RunNumber", pipeline.RunNumber, 1)
	testkit.AssertEqual(t, "pipeline.DurationInSeconds", pipeline.DurationInSeconds, 586)
	testkit.AssertEqual(t, "pipeline.BuildSecondsUsed", pipeline.BuildSecondsUsed, 1113)
	testkit.AssertEqual(t, "pipeline.Duration()", pipeline.Duration(), 586*time.Second)
	testkit.AssertEqual(t, "pipeline.Trigger.Name", pipeline.Trigger.Name, "PUSH")
	testkit.AssertEqual(t, "pipeline.State.IsCompleted()", pipeline.State.IsCompleted(), true)
	testkit.AssertEqual(t, "pipeline.State.Summary()", pipeline.State.Summary(), "STOPPED")
	testkit.AssertEqual(t, "pipeline.HTMLURL()", pipeline.HTMLURL(),
		"https://bitbucket.org/acme/api-gateway/pipelines/results/10471")

	// Pipeline timestamps use Z with 1-9 fractional digits.
	assertTime(t, "pipeline.CreatedOn", pipeline.CreatedOn, "2026-09-03T16:23:58.006085700")
	assertTime(t, "pipeline.CompletedOn", pipeline.CompletedOn, "2026-09-03T16:34:20.692470166")

	if pipeline.State.Stage != nil {
		t.Errorf("pipeline.State.Stage = %+v, want nil on a COMPLETED run", pipeline.State.Stage)
	}
	if pipeline.State.Result == nil {
		t.Fatal("pipeline.State.Result = nil, want the STOPPED result")
	}
	if pipeline.State.Result.Terminator == nil {
		t.Fatal("pipeline.State.Result.Terminator = nil, want the user who stopped the run")
	}
	testkit.AssertEqual(t, "terminator.Name()", pipeline.State.Result.Terminator.Name(), "Anastasiia")

	// The creator can be a bare {type, uuid} stub on legacy runs.
	if pipeline.Creator == nil {
		t.Fatal("pipeline.Creator = nil, want the stub account")
	}
	testkit.AssertEqual(t, "creator.Name()", pipeline.Creator.Name(),
		"{42be8e4e-1f37-474c-8550-9da23cb86f19}")

	testkit.AssertEqual(t, "target.Type", pipeline.Target.Type, "pipeline_pullrequest_target")
	testkit.AssertEqual(t, "target.BranchName()", pipeline.Target.BranchName(),
		"bugfix/RND-13181-upload-validation")
	testkit.AssertEqual(t, "target.Destination", pipeline.Target.Destination, "develop")

	if pipeline.Target.Selector == nil {
		t.Fatal("target.Selector = nil, want the pull-requests selector")
	}
	testkit.AssertEqual(t, "selector.Type", pipeline.Target.Selector.Type, "pull-requests")

	if pipeline.Target.Commit == nil || pipeline.Target.DestinationCommit == nil {
		t.Fatal("target commits = nil, want both head and base")
	}
	testkit.AssertEqual(t, "len(target.Commit.Hash)", len(pipeline.Target.Commit.Hash), 40)
	testkit.AssertEqual(t, "len(target.DestinationCommit.Hash)",
		len(pipeline.Target.DestinationCommit.Hash), 12)

	if pipeline.Target.PullRequest == nil {
		t.Fatal("target.PullRequest = nil, want the PR back-reference")
	}
	testkit.AssertEqual(t, "target.PullRequest.ID", pipeline.Target.PullRequest.ID, 9)
	testkit.AssertEqual(t, "target.PullRequest.Links.HTML.Href",
		pipeline.Target.PullRequest.Links.HTML.Href,
		"https://bitbucket.org/acme/api-gateway/pull-requests/9")

	if pipeline.Repository == nil {
		t.Fatal("pipeline.Repository = nil, want the abbreviated repository")
	}
	testkit.AssertEqual(t, "pipeline.Repository.Workspace()", pipeline.Repository.Workspace(),
		testWorkspace)
	testkit.AssertEqual(t, "pipeline.Links.Steps.Href", pipeline.Links.Steps.Href,
		"https://api.bitbucket.org/2.0/repositories/%7Bws%7D/%7Brepo%7D/pipelines/%7B210f1f44%7D/steps")
}

func TestPipeline_HTMLURLWithoutRepository(t *testing.T) {
	t.Parallel()

	// A pipeline carries no html link at all — links.self uses UUID paths — so
	// the browser URL is built from repository.full_name, and there is nothing to
	// build when the run arrived without one.
	tests := []struct {
		name     string
		pipeline Pipeline
		want     string
	}{
		{
			name:     "no repository",
			pipeline: Pipeline{BuildNumber: 42},
			want:     "",
		},
		{
			name:     "repository without full_name",
			pipeline: Pipeline{BuildNumber: 42, Repository: &RepositoryRef{Name: "API Gateway"}},
			want:     "",
		},
		{
			name:     "repository with full_name",
			pipeline: Pipeline{BuildNumber: 42, Repository: &RepositoryRef{FullName: testRepoFull}},
			want:     "https://bitbucket.org/acme/api-gateway/pipelines/results/42",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "HTMLURL()", test.pipeline.HTMLURL(), test.want)
		})
	}
}

func TestPipelineTarget_BranchName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target PipelineTarget
		want   string
	}{
		{
			name:   "ref target uses ref_name",
			target: PipelineTarget{Type: "pipeline_ref_target", RefType: "branch", RefName: "master"},
			want:   "master",
		},
		{
			name:   "pull request target uses source",
			target: PipelineTarget{Type: "pipeline_pullrequest_target", Source: "feature/x", Destination: "develop"},
			want:   "feature/x",
		},
		{
			name:   "commit target has neither",
			target: PipelineTarget{Type: "pipeline_commit_target"},
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "BranchName()", tc.target.BranchName(), tc.want)
		})
	}
}

func TestPipelineState_Summary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state PipelineState
		want  string
	}{
		{
			name:  "pending has no stage or result",
			state: PipelineState{Name: "PENDING", Type: "pipeline_state_pending"},
			want:  "PENDING",
		},
		{
			name: "in progress shows the stage",
			state: PipelineState{
				Name:  "IN_PROGRESS",
				Type:  "pipeline_state_in_progress",
				Stage: &PipelineStateStage{Type: "pipeline_state_in_progress_running", Name: "RUNNING"},
			},
			want: "RUNNING",
		},
		{
			name: "in progress without a stage falls back to the state name",
			state: PipelineState{
				Name: "IN_PROGRESS",
				Type: "pipeline_state_in_progress",
			},
			want: "IN_PROGRESS",
		},
		{
			name: "completed shows the result",
			state: PipelineState{
				Name:   "COMPLETED",
				Type:   "pipeline_state_completed",
				Result: &PipelineStateResult{Name: "FAILED", Type: "pipeline_state_completed_failed"},
			},
			want: "FAILED",
		},
		{
			name:  "completed without a result falls back to the state name",
			state: PipelineState{Name: "COMPLETED", Type: "pipeline_state_completed"},
			want:  "COMPLETED",
		},
		{
			name:  "unknown state passes through",
			state: PipelineState{Name: "HALTED"},
			want:  "HALTED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "Summary()", tc.state.Summary(), tc.want)
			testkit.AssertEqual(t, "IsCompleted()", tc.state.IsCompleted(), tc.state.Name == "COMPLETED")
		})
	}
}

func TestPipelineStep_UnmarshalFailedStep(t *testing.T) {
	t.Parallel()

	step := decodeJSON[PipelineStep](t, pipelineStepFailedJSON)

	testkit.AssertEqual(t, "step.Name", step.Name, "Cancel Previous Pipelines")
	testkit.AssertEqual(t, "step.DisplayName(0)", step.DisplayName(0), "Cancel Previous Pipelines")
	testkit.AssertEqual(t, "step.Duration()", step.Duration(), 35*time.Second)
	testkit.AssertEqual(t, "step.MaxTime", step.MaxTime, 1)
	testkit.AssertEqual(t, "step.Image.Name", step.Image.Name, "alpine:latest")
	testkit.AssertEqual(t, "step.Trigger.Type", step.Trigger.Type, "pipeline_step_trigger_automatic")
	testkit.AssertEqual(t, "step.Pipeline.UUID", step.Pipeline.UUID,
		"{210f1f44-67d9-4b63-b755-7f46d0af56d4}")
	testkit.AssertEqual(t, "step.State.Summary()", step.State.Summary(), "FAILED")
	assertTime(t, "step.StartedOn", step.StartedOn, "2026-07-02T08:49:39.304503031")
	assertTime(t, "step.CompletedOn", step.CompletedOn, "2026-07-02T08:50:14.413186530")

	if step.State.Result == nil || step.State.Result.Error == nil {
		t.Fatal("step.State.Result.Error = nil, want the git-branch-not-found error")
	}
	testkit.AssertEqual(t, "error.Key", step.State.Result.Error.Key, "agent.git.git-branch-not-found")
	testkit.AssertEqual(t, "error.Message", step.State.Result.Error.Message, "The branch was not found")

	if len(step.ScriptCommands) != 1 || len(step.TeardownCommands) != 1 {
		t.Fatalf("commands = %d script, %d teardown; want 1 and 1",
			len(step.ScriptCommands), len(step.TeardownCommands))
	}
	testkit.AssertEqual(t, "script[0].CommandType", step.ScriptCommands[0].CommandType, "user")
	testkit.AssertEqual(t, "teardown[0].CommandType", step.TeardownCommands[0].CommandType, "system")
	testkit.AssertEqual(t, "teardown[0].Action", step.TeardownCommands[0].Action, "UPLOAD_METRICS")
}

func TestPipelineStep_UnmarshalLegacyStep(t *testing.T) {
	t.Parallel()

	step := decodeJSON[PipelineStep](t, pipelineStepLegacyJSON)

	// A 2017-era step carries no name and no teardown_commands.
	testkit.AssertEqual(t, "step.Name", step.Name, "")
	testkit.AssertEqual(t, "step.DisplayName(0)", step.DisplayName(0), "Step 1")
	testkit.AssertEqual(t, "step.DisplayName(3)", step.DisplayName(3), "Step 4")
	testkit.AssertEqual(t, "step.TeardownCommands", len(step.TeardownCommands), 0)
	testkit.AssertEqual(t, "step.MaxTime", step.MaxTime, 120)
	testkit.AssertEqual(t, "step.Image.Name", step.Image.Name, "debian:8.6")
	testkit.AssertEqual(t, "step.State.Summary()", step.State.Summary(), "STOPPED")
	testkit.AssertEqual(t, "step.Duration()", step.Duration(), 14*time.Second)
	assertTime(t, "step.StartedOn", step.StartedOn, "2017-11-02T17:10:22.427000000")
	testkit.AssertEqual(t, "setup[0].Name", step.SetupCommands[0].Name, "Setting up volume permissions")
	testkit.AssertEqual(t, "setup[0].Command", step.SetupCommands[0].Command, "umask 000")
}

func TestPipelineStepState_Summary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state PipelineStepState
		want  string
	}{
		{
			name:  "pending",
			state: PipelineStepState{Name: "PENDING"},
			want:  "PENDING",
		},
		{
			name:  "in progress",
			state: PipelineStepState{Name: "IN_PROGRESS"},
			want:  "IN_PROGRESS",
		},
		{
			name: "completed and not run",
			state: PipelineStepState{
				Name:   "COMPLETED",
				Result: &PipelineStepResult{Name: "NOT_RUN"},
			},
			want: "NOT_RUN",
		},
		{
			name:  "completed without a result",
			state: PipelineStepState{Name: "COMPLETED"},
			want:  "COMPLETED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "Summary()", tc.state.Summary(), tc.want)
		})
	}
}

func TestTimestampFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
		zero bool
	}{
		{
			name: "six digit fraction with +00:00 offset (pull request)",
			raw:  `"2026-09-03T16:23:55.597480+00:00"`,
			want: "2026-09-03T16:23:55.597480000",
		},
		{
			name: "no fraction with +00:00 offset (commit date)",
			raw:  `"2026-09-03T09:28:54+00:00"`,
			want: "2026-09-03T09:28:54.000000000",
		},
		{
			name: "one digit fraction with Z (2020 pipeline)",
			raw:  `"2020-09-23T14:03:16.2Z"`,
			want: "2020-09-23T14:03:16.200000000",
		},
		{
			name: "two digit fraction with Z",
			raw:  `"2020-04-28T10:34:39.38Z"`,
			want: "2020-04-28T10:34:39.380000000",
		},
		{
			name: "three digit fraction with Z (2017 step)",
			raw:  `"2017-11-02T17:10:13.494Z"`,
			want: "2017-11-02T17:10:13.494000000",
		},
		{
			name: "five digit fraction with Z",
			raw:  `"2021-12-07T17:05:17.78578Z"`,
			want: "2021-12-07T17:05:17.785780000",
		},
		{
			name: "seven digit fraction with Z",
			raw:  `"2026-09-03T16:23:58.0060857Z"`,
			want: "2026-09-03T16:23:58.006085700",
		},
		{
			name: "nine digit fraction with Z",
			raw:  `"2026-09-03T16:34:20.692470166Z"`,
			want: "2026-09-03T16:34:20.692470166",
		},
		{
			name: "non-utc offset",
			raw:  `"2026-09-03T18:34:20.5+02:00"`,
			want: "2026-09-03T16:34:20.500000000",
		},
		{
			name: "json null leaves the zero value",
			raw:  `null`,
			zero: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload := `{"at": ` + tc.raw + `}`
			decoded := decodeJSON[struct {
				At time.Time `json:"at"`
			}](t, payload)

			if tc.zero {
				if !decoded.At.IsZero() {
					t.Errorf("At = %v, want the zero time", decoded.At)
				}
				return
			}
			assertTime(t, "At", decoded.At, tc.want)
		})
	}
}

func TestTimestampAbsentKeyLeavesZero(t *testing.T) {
	t.Parallel()

	// completed_on is absent while a pipeline is still running.
	pipeline := decodeJSON[Pipeline](t, `{"build_number": 3, "state": {"name": "IN_PROGRESS"}}`)

	if !pipeline.CompletedOn.IsZero() {
		t.Errorf("pipeline.CompletedOn = %v, want the zero time", pipeline.CompletedOn)
	}
	testkit.AssertEqual(t, "pipeline.State.Summary()", pipeline.State.Summary(), "IN_PROGRESS")
	testkit.AssertEqual(t, "pipeline.Duration()", pipeline.Duration(), time.Duration(0))
}

func TestSameCommit(t *testing.T) {
	t.Parallel()

	const full = "861daec308a82db4781754d2feea3ee0e6d2c7e2"

	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "identical full hashes", a: full, b: full, want: true},
		{name: "abbreviated matches full", a: "861daec308a8", b: full, want: true},
		{name: "full matches abbreviated", a: full, b: "861daec308a8", want: true},
		{name: "case insensitive", a: "861DAEC308A8", b: full, want: true},
		{name: "seven chars is the shortest accepted", a: "861daec", b: full, want: true},
		{name: "six chars is too short", a: "861dae", b: full, want: false},
		{name: "different commits", a: "87d7f78f5461", b: full, want: false},
		{name: "empty left", a: "", b: full, want: false},
		{name: "empty right", a: full, b: "", want: false},
		{name: "both empty", a: "", b: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "SameCommit()", SameCommit(tc.a, tc.b), tc.want)
		})
	}
}

func TestLatestStatuses(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.September, 3, 14, 0, 0, 0, time.UTC)
	statuses := []CommitStatus{
		{Key: "prs:**:develop", State: StatusInProgress, UpdatedOn: base},
		{Key: "prs:**:release", State: StatusStopped, UpdatedOn: base.Add(3 * time.Minute)},
		{Key: "prs:**:develop", State: StatusSuccessful, UpdatedOn: base.Add(5 * time.Minute)},
		{Key: "prs:**:develop", State: StatusFailed, UpdatedOn: base.Add(1 * time.Minute)},
	}

	latest := LatestStatuses(statuses)

	if len(latest) != 2 {
		t.Fatalf("len(LatestStatuses) = %d, want 2 (one per key)", len(latest))
	}
	// Newest first, and the newest entry per key wins.
	testkit.AssertEqual(t, "latest[0].Key", latest[0].Key, "prs:**:develop")
	testkit.AssertEqual(t, "latest[0].State", latest[0].State, StatusSuccessful)
	testkit.AssertEqual(t, "latest[1].Key", latest[1].Key, "prs:**:release")
	testkit.AssertEqual(t, "latest[1].State", latest[1].State, StatusStopped)

	// The input is not reordered.
	testkit.AssertEqual(t, "statuses[0].State", statuses[0].State, StatusInProgress)

	if got := LatestStatuses(nil); got != nil {
		t.Errorf("LatestStatuses(nil) = %v, want nil", got)
	}
	if got := LatestStatuses([]CommitStatus{}); got != nil {
		t.Errorf("LatestStatuses(empty) = %v, want nil", got)
	}
}

func TestRollupStatuses(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.September, 3, 14, 0, 0, 0, time.UTC)
	newest := base.Add(time.Hour)

	tests := []struct {
		name     string
		statuses []CommitStatus
		want     CommitStatusState
	}{
		{
			name:     "no statuses",
			statuses: nil,
			want:     "",
		},
		{
			name:     "single success",
			statuses: []CommitStatus{{Key: "a", State: StatusSuccessful, UpdatedOn: base}},
			want:     StatusSuccessful,
		},
		{
			name: "failed beats everything",
			statuses: []CommitStatus{
				{Key: "a", State: StatusSuccessful, UpdatedOn: base},
				{Key: "b", State: StatusInProgress, UpdatedOn: base},
				{Key: "c", State: StatusStopped, UpdatedOn: base},
				{Key: "d", State: StatusFailed, UpdatedOn: base},
			},
			want: StatusFailed,
		},
		{
			name: "in progress beats stopped and successful",
			statuses: []CommitStatus{
				{Key: "a", State: StatusSuccessful, UpdatedOn: base},
				{Key: "b", State: StatusStopped, UpdatedOn: base},
				{Key: "c", State: StatusInProgress, UpdatedOn: base},
			},
			want: StatusInProgress,
		},
		{
			name: "stopped beats successful",
			statuses: []CommitStatus{
				{Key: "a", State: StatusSuccessful, UpdatedOn: base},
				{Key: "b", State: StatusStopped, UpdatedOn: base},
			},
			want: StatusStopped,
		},
		{
			name: "a superseded failure on the same key does not count",
			statuses: []CommitStatus{
				{Key: "a", State: StatusFailed, UpdatedOn: base},
				{Key: "a", State: StatusSuccessful, UpdatedOn: newest},
			},
			want: StatusSuccessful,
		},
		{
			name:     "unknown third-party state rolls up as successful",
			statuses: []CommitStatus{{Key: "a", State: "QUEUED", UpdatedOn: base}},
			want:     StatusSuccessful,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "RollupStatuses()", RollupStatuses(tc.statuses), tc.want)
		})
	}
}

func TestWorkspace_Unmarshal(t *testing.T) {
	t.Parallel()

	workspace := decodeJSON[Workspace](t, `{
      "type": "workspace",
      "uuid": "{1211431e-b8a4-44b1-a3c1-5e8721b99a27}",
      "slug": "acme",
      "name": "Acme Corp",
      "is_private": true,
      "created_on": "2017-05-21T09:12:33.123456+00:00",
      "links": {
        "self": {"href": "https://api.bitbucket.org/2.0/workspaces/acme"},
        "html": {"href": "https://bitbucket.org/acme/"},
        "avatar": {"href": "https://bitbucket.org/workspaces/acme/avatar/"}
      }
    }`)

	testkit.AssertEqual(t, "workspace.Slug", workspace.Slug, testWorkspace)
	testkit.AssertEqual(t, "workspace.Name", workspace.Name, "Acme Corp")
	testkit.AssertEqual(t, "workspace.IsPrivate", workspace.IsPrivate, true)
	testkit.AssertEqual(t, "workspace.Links.HTML.Href", workspace.Links.HTML.Href,
		"https://bitbucket.org/acme/")
	assertTime(t, "workspace.CreatedOn", workspace.CreatedOn, "2017-05-21T09:12:33.123456000")
}
