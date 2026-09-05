package main

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui"
)

// pkg/internal/testkit is not importable from cmd/, so this is a local copy of
// testkit.AssertEqual.
func assertEqual[T comparable](t *testing.T, label string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

const (
	testWorkspace = "acme"
	testToken     = "tok"
	testEmail     = "user@acme.com"
	bearerPrefix  = "Bearer "
	basicPrefix   = "Basic "
	readOnlyFlag  = "--read-only"
)

// clearBitbucketEnv drops every BITBUCKET_* override so the ambient
// environment cannot leak into config.Load. The config package keeps its own
// copy of this helper; the env var names are spelled out because its constants
// are unexported.
func clearBitbucketEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"BITBUCKET_WORKSPACE", "BITBUCKET_REPOSITORIES",
		"BITBUCKET_EMAIL", "BITBUCKET_TOKEN", "BITBUCKET_API_URL",
	} {
		t.Setenv(key, "")
	}
}

// basicHeader is the header makeClient builds for an email + token pair.
func basicHeader(email, token string) string {
	return basicPrefix + base64.StdEncoding.EncodeToString([]byte(email+":"+token))
}

// stubAPI answers the two endpoints the connection test calls, with the given
// statuses. Any status >= 400 is served as a real Bitbucket error envelope.
func stubAPI(t *testing.T, workspaceStatus, userStatus int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/workspaces/", func(w http.ResponseWriter, _ *http.Request) {
		writeStub(w, workspaceStatus, `{"type":"workspace","uuid":"{w-1}","slug":"acme","name":"Acme Corp"}`)
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		writeStub(w, userStatus, `{"type":"user","uuid":"{u-1}","display_name":"Demo User","nickname":"demo"}`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func writeStub(w http.ResponseWriter, status int, body string) {
	if status >= http.StatusBadRequest {
		body = `{"type":"error","error":{"message":"` + http.StatusText(status) + `"}}`
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// stubConfig points the client at the stub server.
func stubConfig(baseURL string) *config.Config {
	cfg := &config.Config{}
	cfg.Bitbucket.BaseURL = baseURL
	return cfg
}

func TestParseFlags_CollectsRepeatedRepos(t *testing.T) {
	t.Parallel()
	opts, err := parseFlags([]string{
		"--demo", "--workspace", testWorkspace,
		"--repo", "api-gateway", "--repo", "web-app",
		"--log", "api.log", "--debug", "debug.log",
	}, io.Discard)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	assertEqual(t, "demo", opts.demo, true)
	assertEqual(t, "workspace", opts.workspace, testWorkspace)
	assertEqual(t, "log file", opts.logFile, "api.log")
	assertEqual(t, "debug file", opts.debugFile, "debug.log")
	assertEqual(t, "repos", strings.Join(opts.repos, ","), "api-gateway,web-app")
}

func TestParseFlags_UnknownFlagErrors(t *testing.T) {
	t.Parallel()
	if _, err := parseFlags([]string{"--nope"}, io.Discard); err == nil {
		t.Error("expected an error for an unknown flag")
	}
}

func TestParseFlags_EmptyRepoErrors(t *testing.T) {
	t.Parallel()
	if _, err := parseFlags([]string{"--repo", "  "}, io.Discard); err == nil {
		t.Error("expected an error for an empty --repo value")
	}
}

func TestCliOptions_ApplyToOverridesConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cfg.Bitbucket.Workspace = "configured"
	cfg.Bitbucket.Repositories = []string{"configured-repo"}

	opts := &cliOptions{workspace: testWorkspace, repos: repeatedFlag{"infra"}}
	opts.applyTo(cfg)

	assertEqual(t, "workspace", cfg.Bitbucket.Workspace, testWorkspace)
	assertEqual(t, "repositories", strings.Join(cfg.Bitbucket.Repositories, ","), "infra")
}

func TestCliOptions_ApplyToKeepsConfigWhenFlagsUnset(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cfg.Bitbucket.Workspace = "configured"
	cfg.Bitbucket.Repositories = []string{"configured-repo"}

	(&cliOptions{}).applyTo(cfg)

	assertEqual(t, "workspace", cfg.Bitbucket.Workspace, "configured")
	assertEqual(t, "repositories", strings.Join(cfg.Bitbucket.Repositories, ","), "configured-repo")
}

func TestParseFlags_ReadOnly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"absent", nil, false},
		{"present", []string{readOnlyFlag}, true},
		{"explicitly true", []string{readOnlyFlag + "=true"}, true},
		{"explicitly false", []string{readOnlyFlag + "=false"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, err := parseFlags(tc.args, io.Discard)
			if err != nil {
				t.Fatalf("parseFlags: %v", err)
			}
			assertEqual(t, "readOnly", opts.readOnly, tc.want)
		})
	}
}

// The flag is a one-way switch: it can only turn read-only mode on, so a
// configured or environment-set read-only session cannot be widened from the
// command line.
func TestCliOptions_ApplyToReadOnlyOnlyTurnsItOn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		flag       bool
		configured bool
		want       bool
	}{
		{"flag on, config off", true, false, true},
		{"flag off, config on stays on", false, true, true},
		{"flag on, config on", true, true, true},
		{"neither", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{}
			cfg.Bitbucket.ReadOnly = tc.configured

			(&cliOptions{readOnly: tc.flag}).applyTo(cfg)

			assertEqual(t, "Bitbucket.ReadOnly", cfg.Bitbucket.ReadOnly, tc.want)
		})
	}
}

func TestMakeClient_ReadOnlyFollowsTheConfig(t *testing.T) {
	t.Parallel()
	assertEqual(t, "default", makeClient(&config.Config{}, authBearer, "", testToken).ReadOnly(), false)

	cfg := &config.Config{}
	cfg.Bitbucket.ReadOnly = true
	assertEqual(t, "read-only", makeClient(cfg, authBearer, "", testToken).ReadOnly(), true)
}

func TestResolveClient_ReadOnlyReachesTheSavedCredentialsClient(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	if err := config.SaveCredentials(&config.Credentials{
		AuthMethod: authBearer, Token: testToken, Workspace: testWorkspace,
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	cfg := &config.Config{}
	cfg.Bitbucket.ReadOnly = true
	client, _, err := resolveClient(cfg)
	if err != nil {
		t.Fatalf("resolveClient: %v", err)
	}
	assertEqual(t, "ReadOnly", client.ReadOnly(), true)
}

func TestMakeClient_AuthHeaderPerMethod(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		authMethod string
		email      string
		want       string
	}{
		{"explicit bearer ignores a stray email", authBearer, testEmail, bearerPrefix},
		{"explicit basic", authBasic, testEmail, basicPrefix},
		{"unset method with an email is basic", "", testEmail, basicPrefix},
		{"unset method without an email is bearer", "", "", bearerPrefix},
	}
	for _, tc := range cases {
		client := makeClient(&config.Config{}, tc.authMethod, tc.email, testToken)
		if !strings.HasPrefix(client.AuthHeader(), tc.want) {
			t.Errorf("%s: auth header = %q, want prefix %q", tc.name, client.AuthHeader(), tc.want)
		}
	}
}

func TestMakeClient_UsesConfiguredBaseURL(t *testing.T) {
	t.Parallel()
	assertEqual(t, "default base URL",
		makeClient(&config.Config{}, authBearer, "", testToken).BaseURL(), config.DefaultBaseURL)
	assertEqual(t, "configured base URL",
		makeClient(stubConfig("https://bitbucket.example.com/2.0/"), authBearer, "", testToken).BaseURL(),
		"https://bitbucket.example.com/2.0")
}

func TestTestConnection_ReportsCurrentUser(t *testing.T) {
	t.Parallel()
	server := stubAPI(t, http.StatusOK, http.StatusOK)
	client := makeClient(stubConfig(server.URL), authBearer, "", testToken)

	var out strings.Builder
	if err := testConnection(t.Context(), client, testWorkspace, &out); err != nil {
		t.Fatalf("testConnection: %v", err)
	}
	if !strings.Contains(out.String(), "Connected as Demo User") {
		t.Errorf("output = %q, want the display name", out.String())
	}
}

func TestTestConnection_TokenWithoutUserAccessStillConnects(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		server := stubAPI(t, http.StatusOK, status)
		client := makeClient(stubConfig(server.URL), authBearer, "", testToken)

		var out strings.Builder
		if err := testConnection(t.Context(), client, testWorkspace, &out); err != nil {
			t.Fatalf("testConnection with /user %d: %v", status, err)
		}
		if !strings.Contains(out.String(), "Mine/Reviewing tabs are disabled") {
			t.Errorf("output for /user %d = %q, want the disabled-tabs note", status, out.String())
		}
	}
}

func TestTestConnection_UnknownUserFailureErrors(t *testing.T) {
	t.Parallel()
	server := stubAPI(t, http.StatusOK, http.StatusInternalServerError)
	client := makeClient(stubConfig(server.URL), authBearer, "", testToken)

	err := testConnection(t.Context(), client, testWorkspace, io.Discard)
	if err == nil {
		t.Fatal("expected an error for a 500 from /user")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("error = %q, want it to mention status 500", err)
	}
}

func TestTestConnection_MissingWorkspaceErrors(t *testing.T) {
	t.Parallel()
	server := stubAPI(t, http.StatusNotFound, http.StatusOK)
	client := makeClient(stubConfig(server.URL), authBearer, "", testToken)

	err := testConnection(t.Context(), client, testWorkspace, io.Discard)
	if err == nil {
		t.Fatal("expected an error for a 404 workspace")
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("error = %q, want it to mention status 404", err)
	}
}

func TestResolveClient_SavedBearerCredentials(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	if err := config.SaveCredentials(&config.Credentials{
		AuthMethod: authBearer, Token: testToken, Workspace: testWorkspace,
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	cfg := &config.Config{}
	client, method, err := resolveClient(cfg)
	if err != nil {
		t.Fatalf("resolveClient: %v", err)
	}
	assertEqual(t, "auth method", method, tui.AuthSaved)
	assertEqual(t, "workspace", cfg.Bitbucket.Workspace, testWorkspace)
	if !strings.HasPrefix(client.AuthHeader(), bearerPrefix) {
		t.Errorf("auth header = %q, want a Bearer prefix", client.AuthHeader())
	}
}

func TestResolveClient_SavedBasicCredentials(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	if err := config.SaveCredentials(&config.Credentials{
		AuthMethod: authBasic, Email: testEmail, Token: testToken, Workspace: testWorkspace,
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	cfg := &config.Config{}
	client, method, err := resolveClient(cfg)
	if err != nil {
		t.Fatalf("resolveClient: %v", err)
	}
	assertEqual(t, "auth method", method, tui.AuthSaved)
	assertEqual(t, "email", cfg.Bitbucket.Email, testEmail)
	if !strings.HasPrefix(client.AuthHeader(), basicPrefix) {
		t.Errorf("auth header = %q, want a Basic prefix", client.AuthHeader())
	}
}

func TestResolveClient_ConfiguredWorkspaceWinsOverSavedOne(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	if err := config.SaveCredentials(&config.Credentials{
		AuthMethod: authBearer, Token: testToken, Workspace: "saved-workspace",
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	cfg := &config.Config{}
	cfg.Bitbucket.Workspace = testWorkspace
	if _, _, err := resolveClient(cfg); err != nil {
		t.Fatalf("resolveClient: %v", err)
	}
	assertEqual(t, "workspace", cfg.Bitbucket.Workspace, testWorkspace)
}

func TestResolveClient_EnvTokenWithoutEmailUsesBearer(t *testing.T) {
	clearBitbucketEnv(t)
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	t.Setenv("BITBUCKET_WORKSPACE", testWorkspace)
	t.Setenv("BITBUCKET_TOKEN", testToken)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	client, method, err := resolveClient(cfg)
	if err != nil {
		t.Fatalf("resolveClient: %v", err)
	}
	assertEqual(t, "auth method", method, tui.AuthEnv)
	if !strings.HasPrefix(client.AuthHeader(), bearerPrefix) {
		t.Errorf("auth header = %q, want a Bearer prefix", client.AuthHeader())
	}
}

func TestResolveClient_EnvTokenWithEmailUsesBasic(t *testing.T) {
	clearBitbucketEnv(t)
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	t.Setenv("BITBUCKET_WORKSPACE", testWorkspace)
	t.Setenv("BITBUCKET_TOKEN", testToken)
	t.Setenv("BITBUCKET_EMAIL", testEmail)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	client, method, err := resolveClient(cfg)
	if err != nil {
		t.Fatalf("resolveClient: %v", err)
	}
	assertEqual(t, "auth method", method, tui.AuthEnv)
	if !strings.HasPrefix(client.AuthHeader(), basicPrefix) {
		t.Errorf("auth header = %q, want a Basic prefix", client.AuthHeader())
	}
}

func TestResolveClient_EnvTokenWinsOverSavedCredentials(t *testing.T) {
	clearBitbucketEnv(t)
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	if err := config.SaveCredentials(&config.Credentials{
		AuthMethod: authBearer, Token: "saved-token", Workspace: "saved-workspace",
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	t.Setenv("BITBUCKET_TOKEN", testToken)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	client, method, err := resolveClient(cfg)
	if err != nil {
		t.Fatalf("resolveClient: %v", err)
	}
	assertEqual(t, "auth method", method, tui.AuthEnv)
	assertEqual(t, "auth header", client.AuthHeader(), bearerPrefix+testToken)
	// The saved workspace still fills the gap the environment leaves.
	assertEqual(t, "workspace", cfg.Bitbucket.Workspace, "saved-workspace")
}

func TestResolveClient_EnvEmailWinsOverSavedEmail(t *testing.T) {
	clearBitbucketEnv(t)
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	if err := config.SaveCredentials(&config.Credentials{
		AuthMethod: authBasic, Email: "saved@acme.com", Token: "saved-token", Workspace: testWorkspace,
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	t.Setenv("BITBUCKET_TOKEN", testToken)
	t.Setenv("BITBUCKET_EMAIL", testEmail)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	client, method, err := resolveClient(cfg)
	if err != nil {
		t.Fatalf("resolveClient: %v", err)
	}
	assertEqual(t, "auth method", method, tui.AuthEnv)
	assertEqual(t, "auth header", client.AuthHeader(), basicHeader(testEmail, testToken))
	assertEqual(t, "email", cfg.Bitbucket.Email, testEmail)
}

func TestResolveClient_SavedCredentialsWinWithoutAnEnvToken(t *testing.T) {
	clearBitbucketEnv(t)
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	if err := config.SaveCredentials(&config.Credentials{
		AuthMethod: authBasic, Email: testEmail, Token: testToken, Workspace: testWorkspace,
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	client, method, err := resolveClient(cfg)
	if err != nil {
		t.Fatalf("resolveClient: %v", err)
	}
	assertEqual(t, "auth method", method, tui.AuthSaved)
	assertEqual(t, "auth header", client.AuthHeader(), basicHeader(testEmail, testToken))
	assertEqual(t, "workspace", cfg.Bitbucket.Workspace, testWorkspace)
}

func TestResolveClient_WorkspaceRequired(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	if err := config.SaveCredentials(&config.Credentials{
		AuthMethod: authBearer, Token: testToken,
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	_, _, err := resolveClient(&config.Config{})
	if !errors.Is(err, errWorkspaceRequired) {
		t.Fatalf("error = %v, want errWorkspaceRequired", err)
	}
	if !strings.Contains(err.Error(), "BITBUCKET_WORKSPACE") {
		t.Errorf("error = %q, want it to name the env var", err)
	}
}

func TestRunSetupWizard_BearerSuccessSavesCredentials(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	server := stubAPI(t, http.StatusOK, http.StatusOK)

	cfg := stubConfig(server.URL)
	client, err := runSetupWizard(cfg, strings.NewReader("1\n"+testToken+"\n"+testWorkspace+"\n"))
	if err != nil {
		t.Fatalf("runSetupWizard: %v", err)
	}
	if client == nil {
		t.Fatal("client = nil, want a client")
	}
	if !strings.HasPrefix(client.AuthHeader(), bearerPrefix) {
		t.Errorf("auth header = %q, want a Bearer prefix", client.AuthHeader())
	}
	assertEqual(t, "workspace", cfg.Bitbucket.Workspace, testWorkspace)

	creds, err := config.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds == nil {
		t.Fatal("creds = nil, want saved credentials")
	}
	assertEqual(t, "saved auth method", creds.AuthMethod, authBearer)
	assertEqual(t, "saved token", creds.Token, testToken)
	assertEqual(t, "saved workspace", creds.Workspace, testWorkspace)
	assertEqual(t, "saved email", creds.Email, "")
}

func TestRunSetupWizard_BasicSuccessSavesCredentials(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	server := stubAPI(t, http.StatusOK, http.StatusOK)

	client, err := runSetupWizard(stubConfig(server.URL),
		strings.NewReader("2\n"+testEmail+"\n"+testToken+"\n"+testWorkspace+"\n"))
	if err != nil {
		t.Fatalf("runSetupWizard: %v", err)
	}
	if !strings.HasPrefix(client.AuthHeader(), basicPrefix) {
		t.Errorf("auth header = %q, want a Basic prefix", client.AuthHeader())
	}

	creds, err := config.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds == nil {
		t.Fatal("creds = nil, want saved credentials")
	}
	assertEqual(t, "saved auth method", creds.AuthMethod, authBasic)
	assertEqual(t, "saved email", creds.Email, testEmail)
}

func TestRunSetupWizard_DefaultsToBearerAndConfiguredWorkspace(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	server := stubAPI(t, http.StatusOK, http.StatusUnauthorized)

	cfg := stubConfig(server.URL)
	cfg.Bitbucket.Workspace = testWorkspace
	if _, err := runSetupWizard(cfg, strings.NewReader("\n"+testToken+"\n\n")); err != nil {
		t.Fatalf("runSetupWizard: %v", err)
	}

	creds, err := config.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds == nil {
		t.Fatal("creds = nil, want saved credentials")
	}
	assertEqual(t, "saved auth method", creds.AuthMethod, authBearer)
	assertEqual(t, "saved workspace", creds.Workspace, testWorkspace)
}

// The wizard only reads, so read-only mode must not change what it does: it
// still verifies the connection and saves the credentials, and hands back a
// client that refuses writes.
func TestRunSetupWizard_ReadOnlyLeavesTheWizardUnchanged(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	server := stubAPI(t, http.StatusOK, http.StatusOK)

	cfg := stubConfig(server.URL)
	cfg.Bitbucket.ReadOnly = true
	client, err := runSetupWizard(cfg, strings.NewReader("1\n"+testToken+"\n"+testWorkspace+"\n"))
	if err != nil {
		t.Fatalf("runSetupWizard: %v", err)
	}
	assertEqual(t, "ReadOnly", client.ReadOnly(), true)

	creds, err := config.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds == nil {
		t.Fatal("creds = nil, want saved credentials")
	}
	assertEqual(t, "saved workspace", creds.Workspace, testWorkspace)
}

func TestRunSetupWizard_MissingTokenErrors(t *testing.T) {
	t.Parallel()
	if _, err := runSetupWizard(&config.Config{}, strings.NewReader("1\n\n")); !errors.Is(err, errWizardToken) {
		t.Errorf("error = %v, want errWizardToken", err)
	}
}

func TestRunSetupWizard_MissingEmailErrors(t *testing.T) {
	t.Parallel()
	if _, err := runSetupWizard(&config.Config{}, strings.NewReader("2\n\n")); !errors.Is(err, errWizardEmail) {
		t.Errorf("error = %v, want errWizardEmail", err)
	}
}

func TestRunSetupWizard_MissingWorkspaceErrors(t *testing.T) {
	t.Parallel()
	_, err := runSetupWizard(&config.Config{}, strings.NewReader("1\n"+testToken+"\n\n"))
	if !errors.Is(err, errWizardWorkspace) {
		t.Errorf("error = %v, want errWizardWorkspace", err)
	}
}

func TestRunSetupWizard_ConnectionFailureErrors(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	server := stubAPI(t, http.StatusNotFound, http.StatusOK)

	_, err := runSetupWizard(stubConfig(server.URL),
		strings.NewReader("1\n"+testToken+"\n"+testWorkspace+"\n"))
	if !errors.Is(err, errConnectionFailed) {
		t.Fatalf("error = %v, want errConnectionFailed", err)
	}
	if creds, _ := config.LoadCredentials(); creds != nil {
		t.Error("credentials were saved despite a failed connection test")
	}
}

func TestDispatch_VersionPrints(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"--version", "version"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr strings.Builder
			assertEqual(t, "exit code", dispatch([]string{arg}, &stdout, &stderr), 0)
			if !strings.Contains(stdout.String(), "lazybitbucket ") {
				t.Errorf("version output = %q, want a lazybitbucket prefix", stdout.String())
			}
			assertEqual(t, "stderr", stderr.String(), "")
		})
	}
}

func TestDispatch_LogoutClearsCredentials(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	if err := config.SaveCredentials(&config.Credentials{
		AuthMethod: authBearer, Token: testToken, Workspace: testWorkspace,
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	var stdout, stderr strings.Builder
	assertEqual(t, "exit code", dispatch([]string{"logout"}, &stdout, &stderr), 0)
	if !strings.Contains(stdout.String(), "Credentials cleared") {
		t.Errorf("logout output = %q", stdout.String())
	}
	if creds, _ := config.LoadCredentials(); creds != nil {
		t.Error("credentials should be gone after logout")
	}
}

func TestRunAuth_WizardSuccess(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())
	t.Setenv("BITBUCKET_API_URL", stubAPI(t, http.StatusOK, http.StatusOK).URL)

	if err := runAuth(nil, strings.NewReader("1\n"+testToken+"\n"+testWorkspace+"\n")); err != nil {
		t.Fatalf("runAuth: %v", err)
	}
	creds, err := config.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds == nil {
		t.Fatal("creds = nil, want saved credentials")
	}
	assertEqual(t, "saved workspace", creds.Workspace, testWorkspace)
}

func TestRunAuth_WizardFailureReturnsError(t *testing.T) {
	t.Setenv("LAZYBITBUCKET_CONFIG_DIR", t.TempDir())

	if err := runAuth(nil, strings.NewReader("1\n\n")); !errors.Is(err, errWizardToken) {
		t.Errorf("error = %v, want errWizardToken", err)
	}
}

func TestDispatch_HelpExitsZero(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"--help", "-h"} {
		var stdout, stderr strings.Builder
		code := dispatch([]string{arg}, &stdout, &stderr)
		assertEqual(t, "exit code for "+arg, code, 0)
		if !strings.Contains(stderr.String(), "Usage of lazybitbucket") {
			t.Errorf("stderr for %s = %q, want usage text", arg, stderr.String())
		}
		if strings.Contains(stderr.String(), "Error:") {
			t.Errorf("stderr for %s reports an error: %q", arg, stderr.String())
		}
	}
}

func TestDispatch_HelpListsTheReadOnlyFlag(t *testing.T) {
	t.Parallel()
	var stdout, stderr strings.Builder
	assertEqual(t, "exit code", dispatch([]string{"--help"}, &stdout, &stderr), 0)
	if !strings.Contains(stderr.String(), "-read-only") {
		t.Errorf("usage = %q, want it to list -read-only", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Refuse every request that would modify Bitbucket") {
		t.Errorf("usage = %q, want the -read-only help text", stderr.String())
	}
}
