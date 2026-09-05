package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui"
)

// The two credential shapes the wizard offers. They are the values stored in
// config.Credentials.AuthMethod and understood by makeClient.
const (
	authBearer = "bearer"
	authBasic  = "basic"
)

var (
	errWorkspaceRequired = errors.New(
		"workspace is required (set bitbucket.workspace in config.yml, BITBUCKET_WORKSPACE, " +
			"or run 'lazybitbucket auth')")
	errWizardWorkspace  = errors.New("workspace is required")
	errWizardToken      = errors.New("token is required")
	errWizardEmail      = errors.New("email is required")
	errConnectionFailed = errors.New("connection test failed")
)

// userAgent identifies this build to the Bitbucket API.
func userAgent() string { return "lazybitbucket/" + version }

// makeClient builds a Bitbucket client for the given credentials, refusing
// every write when the configuration asks for read-only (browse-only) mode.
// Basic auth is used when authMethod is "basic", or when it is unset and an
// email is present; every other combination sends the token as a Bearer, so an
// explicit "bearer" ignores a stray email left over in the config file.
func makeClient(cfg *config.Config, authMethod, email, token string) *bitbucket.Client {
	useBasic := authMethod == authBasic || (authMethod == "" && email != "")
	if !useBasic {
		email = ""
	}
	return bitbucket.NewClient(bitbucket.ClientOpts{
		BaseURL:   cfg.ResolveBaseURL(),
		Token:     token,
		Email:     email,
		UserAgent: userAgent(),
		ReadOnly:  cfg.Bitbucket.ReadOnly,
	})
}

// resolveClient finds credentials in the environment and config file, then in
// saved auth.json, then by running the interactive wizard. It reports which of
// those paths won so the UI can show it, and fails when no workspace is known
// by the end.
func resolveClient(cfg *config.Config) (*bitbucket.Client, tui.AuthMethod, error) {
	client, method, err := resolveCredentials(cfg)
	if err != nil {
		return nil, "", err
	}
	if cfg.Bitbucket.Workspace == "" {
		return nil, "", errWorkspaceRequired
	}
	return client, method, nil
}

func resolveCredentials(cfg *config.Config) (*bitbucket.Client, tui.AuthMethod, error) {
	// Saved credentials fill in what the environment and config file leave
	// unset, but never overwrite them.
	creds, err := config.LoadCredentials()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	if creds != nil {
		if cfg.Bitbucket.Workspace == "" {
			cfg.Bitbucket.Workspace = creds.Workspace
		}
		if cfg.Bitbucket.Email == "" {
			cfg.Bitbucket.Email = creds.Email
		}
	}

	// 1. Environment variables and the config file. BITBUCKET_TOKEN is the only
	// writer of cfg.Bitbucket.Token, so a token here was set in the environment,
	// which takes priority over auth.json.
	if cfg.Bitbucket.Token != "" {
		return makeClient(cfg, "", cfg.Bitbucket.Email, cfg.Bitbucket.Token), tui.AuthEnv, nil
	}

	// 2. Saved credentials.
	if creds != nil && creds.Token != "" {
		return makeClient(cfg, creds.AuthMethod, cfg.Bitbucket.Email, creds.Token), tui.AuthSaved, nil
	}

	// 3. Interactive wizard.
	fmt.Println()
	fmt.Println("  Welcome to lazybitbucket! Let's set up your Bitbucket connection.")
	fmt.Println()
	client, err := runSetupWizard(cfg, os.Stdin)
	if err != nil {
		return nil, "", err
	}
	return client, tui.AuthWizard, nil
}

// runSetupWizard interactively collects Bitbucket credentials, verifies them
// and saves them to auth.json.
func runSetupWizard(cfg *config.Config, input io.Reader) (*bitbucket.Client, error) {
	reader := bufio.NewReader(input)

	printTokenHelp()

	fmt.Println("  \033[1mAuthentication\033[0m")
	fmt.Println("  1) API token or access token (sent as Bearer)")
	fmt.Println("  2) Atlassian account email + API token (Basic)")
	fmt.Println()
	fmt.Print("  Choice [1]: ")
	authMethod := authBearer
	if readLine(reader) == "2" {
		authMethod = authBasic
	}
	fmt.Println()

	var email string
	if authMethod == authBasic {
		fmt.Println("  \033[1mEmail\033[0m")
		fmt.Println("  Your Atlassian account email, not your Bitbucket username")
		fmt.Println()
		fmt.Print("  Email: ")
		email = readLine(reader)
		if email == "" {
			return nil, errWizardEmail
		}
		fmt.Println()
	}

	fmt.Println("  \033[1mToken\033[0m")
	fmt.Print("  Token: ")
	token := readLine(reader)
	if token == "" {
		return nil, errWizardToken
	}
	fmt.Println()

	workspace, err := promptWorkspace(reader, cfg.Bitbucket.Workspace)
	if err != nil {
		return nil, err
	}

	cfg.Bitbucket.Workspace = workspace
	cfg.Bitbucket.Email = email

	fmt.Println("  Verifying connection...")
	client := makeClient(cfg, authMethod, email, token)
	if err := testConnection(context.Background(), client, workspace, os.Stdout); err != nil {
		fmt.Printf("\n  \033[31m✗ Connection failed: %v\033[0m\n\n", err)
		fmt.Println("  Please check your credentials and try again.")
		fmt.Println("  Run 'lazybitbucket auth' to retry.")
		return nil, errConnectionFailed
	}
	fmt.Println("  \033[32m✓ Connected successfully!\033[0m")
	fmt.Println()

	saveCredentials(&config.Credentials{
		AuthMethod: authMethod,
		Email:      email,
		Token:      token,
		Workspace:  workspace,
	})
	printRepositoriesHint()

	return client, nil
}

// promptWorkspace asks for the workspace slug, offering the configured one as
// the default.
func promptWorkspace(reader *bufio.Reader, current string) (string, error) {
	fmt.Println("  \033[1mWorkspace\033[0m")
	fmt.Println("  The workspace slug from your Bitbucket URL, e.g. bitbucket.org/\033[4macme\033[0m/repo")
	fmt.Println()
	if current == "" {
		fmt.Print("  Workspace: ")
	} else {
		fmt.Printf("  Workspace [%s]: ", current)
	}
	workspace := readLine(reader)
	if workspace == "" {
		workspace = current
	}
	if workspace == "" {
		return "", errWizardWorkspace
	}
	fmt.Println()
	return workspace, nil
}

// testConnection verifies the credentials. Reading the workspace must succeed —
// it is the one call every credential kind can make. Reading the current user
// is optimistic: repository, project and workspace access tokens are not tied
// to a user and get a 401 or 403 there, which only costs the Mine and
// Reviewing tabs.
func testConnection(ctx context.Context, client *bitbucket.Client, workspace string, out io.Writer) error {
	if _, err := client.GetWorkspace(ctx, workspace); err != nil {
		return err
	}

	user, err := client.GetCurrentUser(ctx)
	switch {
	case err == nil:
		fmt.Fprintf(out, "  Connected as %s\n", user.Name())
	case bitbucket.IsUnauthorized(err), bitbucket.IsForbidden(err):
		fmt.Fprintln(out, "  Connected (token cannot read the current user; Mine/Reviewing tabs are disabled)")
	default:
		return err
	}
	return nil
}

// saveCredentials writes auth.json, reporting where it went. A failure is a
// warning, not an error: the session that just authenticated still works.
func saveCredentials(creds *config.Credentials) {
	if err := config.SaveCredentials(creds); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not save credentials to %s: %v\n", config.AuthPath(), err)
		return
	}
	fmt.Printf("  Credentials saved to %s\n", config.AuthPath())
	fmt.Println()
}

func printTokenHelp() {
	fmt.Println("  \033[1mAPI token\033[0m")
	fmt.Println("  Atlassian account → Security → 'Create and manage API tokens' →")
	fmt.Println("  'Create API token with scopes', app: Bitbucket. Tick these scopes:")
	fmt.Println("    read:pullrequest:bitbucket")
	fmt.Println("    read:repository:bitbucket")
	fmt.Println("    read:pipeline:bitbucket")
	fmt.Println("    read:user:bitbucket")
	fmt.Println("    read:workspace:bitbucket")
	fmt.Println("  Merging and editing a pull request need one more:")
	fmt.Println("    write:pullrequest:bitbucket   (title, description, reviewers, draft, merge)")
	fmt.Println("  Leave it off and every read still works; a write comes back 403. Read-only")
	fmt.Println("  mode (--read-only, or bitbucket.readOnly) does not need it and refuses writes")
	fmt.Println("  before they are sent.")
	fmt.Println("  A repository, project or workspace access token works too; it just cannot")
	fmt.Println("  read the current user, so the Mine and Reviewing tabs stay empty.")
	fmt.Println()
}

func printRepositoriesHint() {
	fmt.Printf("  Repositories are configured in %s under bitbucket.repositories.\n", config.ConfigPath())
	fmt.Println("  When none are configured every repository you can see is listed —")
	fmt.Println("  access tokens must configure them, because that discovery is user-centric")
	fmt.Println("  and returns nothing for a token.")
	fmt.Println()
}

// readLine reads one trimmed line; a read error yields an empty line, which
// every caller already treats as "not provided".
func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// runAuth handles 'lazybitbucket auth' and re-runs the setup wizard.
func runAuth(args []string, input io.Reader) error {
	authFlags := flag.NewFlagSet("auth", flag.ContinueOnError)
	if err := authFlags.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	_, err = runSetupWizard(cfg, input)
	return err
}
