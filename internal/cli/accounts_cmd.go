package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	accountsCmd := &cobra.Command{
		Use:   "accounts",
		Short: "Manage Gmail accounts",
	}

	accountsCmd.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add a Gmail account via OAuth2 (guided setup)",
		RunE:  runAccountsAdd,
	})

	accountsCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all authenticated accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			accs, err := a.accounts.ListAccounts()
			if err != nil {
				return err
			}
			if len(accs) == 0 {
				fmt.Println("No accounts. Run: mailrat accounts add")
				return nil
			}
			for _, acc := range accs {
				fmt.Printf("  %s  (id: %s)\n", acc.Email, acc.ID)
			}
			return nil
		},
	})

	var forceRemove bool

	removeCmd := &cobra.Command{
		Use:   "remove <account-id|email>",
		Short: "Remove an account and delete all its local data",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			query := args[0]

			// Resolve by ID or email.
			accs, err := a.accounts.ListAccounts()
			if err != nil {
				return err
			}

			var matchID, matchEmail string
			for _, acc := range accs {
				if acc.ID == query || acc.Email == query {
					matchID = acc.ID
					matchEmail = acc.Email
					break
				}
			}
			if matchID == "" {
				return fmt.Errorf("no account found matching %q\n\nRun 'mailrat accounts list' to see available accounts.", query)
			}

			if !forceRemove {
				fmt.Printf("Remove account %s and delete ALL local data?\n", matchEmail)
				fmt.Println("This will delete all cached emails, labels, drafts, and contacts for this account.")
				fmt.Print("Type the email address to confirm: ")
				reader := bufio.NewReader(os.Stdin)
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(input)
				if input != matchEmail {
					fmt.Println("Input did not match. Cancelled.")
					return nil
				}
			}

			if err := a.store.DeleteAccount(matchID); err != nil {
				return fmt.Errorf("remove account: %w", err)
			}

			fmt.Printf("✓ Account %s removed. All local data deleted.\n", matchEmail)
			return nil
		},
	}
	removeCmd.Flags().BoolVarP(&forceRemove, "force", "f", false, "Skip confirmation prompt")

	accountsCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(accountsCmd)
}

func runAccountsAdd(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	printHeader("mailrat — Add Gmail Account")

	// -----------------------------------------------------------------------
	// Step 1: Ensure credentials.json exists
	// -----------------------------------------------------------------------
	printStep(1, "Google Cloud credentials")

	credPath, err := CredentialsPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		fmt.Printf("  No credentials.json found at: %s\n\n", credPath)
		fmt.Println("  You need a Google Cloud OAuth2 credentials file.")
		fmt.Println("  Follow these steps to create one:")
		fmt.Println()
		fmt.Println("  a) Go to: https://console.cloud.google.com/")
		fmt.Println()
		fmt.Println("  b) Create a project (or select an existing one).")
		fmt.Println("     Top-left dropdown → 'New Project' → name it anything.")
		fmt.Println()
		fmt.Println("  c) Enable the Gmail API:")
		fmt.Println("     APIs & Services → Library → search 'Gmail API' → Enable")
		fmt.Println()
		fmt.Println("  d) Enable the Google People API (for contacts):")
		fmt.Println("     APIs & Services → Library → search 'People API' → Enable")
		fmt.Println()
		fmt.Println("  e) Create OAuth2 credentials:")
		fmt.Println("     APIs & Services → Credentials → Create Credentials")
		fmt.Println("     → OAuth client ID → Application type: Desktop app")
		fmt.Println("     → Name it anything → Create")
		fmt.Println()
		fmt.Println("  f) Configure the OAuth consent screen (if prompted):")
		fmt.Println("     User type: External → fill in App name + your email")
		fmt.Println("     → Add scopes: Gmail API (all), People API (contacts.readonly)")
		fmt.Println("     → Add your Gmail address as a test user")
		fmt.Println()
		fmt.Println("  g) Download the credentials:")
		fmt.Println("     On the credentials page, click the download icon (⬇) next to")
		fmt.Println("     your OAuth client → saves as credentials.json")
		fmt.Println()
		fmt.Println("  ─────────────────────────────────────────────────────────────────")
		fmt.Println()

		credPath, err = promptCredentialsPath(reader, credPath)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("  ✓ Found credentials.json at: %s\n", credPath)
	}

	// -----------------------------------------------------------------------
	// Step 2: Validate the credentials file
	// -----------------------------------------------------------------------
	printStep(2, "Validate credentials")

	if err := validateCredentials(credPath); err != nil {
		return fmt.Errorf("invalid credentials file: %w\n\n  Make sure you downloaded the OAuth client credentials (not a service account key).", err)
	}
	fmt.Println("  ✓ Credentials file looks valid (OAuth2 Desktop app format)")

	// -----------------------------------------------------------------------
	// Step 3: OAuth consent + token exchange
	// -----------------------------------------------------------------------
	printStep(3, "Authenticate with Google")
	fmt.Println()
	fmt.Println("  mailrat will now request access to your Gmail account.")
	fmt.Println("  You will be asked to sign in and grant the following permissions:")
	fmt.Println("    • Read your emails")
	fmt.Println("    • Modify your emails (labels, archive)")
	fmt.Println("    • Send emails on your behalf")
	fmt.Println("    • Read your Google contacts")
	fmt.Println()
	fmt.Print("  Press Enter to continue...")
	reader.ReadString('\n') //nolint:errcheck
	fmt.Println()

	a, err := newAppWithCredentials(credPath)
	if err != nil {
		return err
	}
	defer a.close()

	acc, err := a.accounts.AddAccount(context.Background())
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// -----------------------------------------------------------------------
	// Done
	// -----------------------------------------------------------------------
	fmt.Println()
	fmt.Println("  ┌─────────────────────────────────────────────┐")
	fmt.Printf( "  │  ✓ Account added: %-26s │\n", acc.Email)
	fmt.Println("  └─────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("    mailrat sync --days=30   # sync the last 30 days of mail")
	fmt.Println("    mailrat inbox list       # view your inbox")
	fmt.Println()

	return nil
}

// promptCredentialsPath asks the user to provide the credentials.json path,
// copies it to the expected location if needed, and returns the final path.
func promptCredentialsPath(reader *bufio.Reader, defaultPath string) (string, error) {
	fmt.Println("  Once you have downloaded credentials.json, you have two options:")
	fmt.Println()
	fmt.Printf("  Option A: Copy the file yourself to:\n    %s\n", defaultPath)
	fmt.Println()
	fmt.Println("  Option B: Enter the path to the downloaded file here and")
	fmt.Println("            mailrat will copy it for you.")
	fmt.Println()
	fmt.Print("  Path to credentials.json (or press Enter to exit and copy manually): ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)

	if input == "" {
		fmt.Println()
		fmt.Printf("  Place credentials.json at:\n    %s\n\n", defaultPath)
		fmt.Println("  Then re-run: mailrat accounts add")
		os.Exit(0)
	}

	// Expand ~ if present.
	if strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		input = filepath.Join(home, input[2:])
	}

	// Validate the source file before copying.
	if err := validateCredentials(input); err != nil {
		return "", fmt.Errorf("file at %q does not look like valid OAuth credentials: %w", input, err)
	}

	// Copy to default location.
	data, err := os.ReadFile(input)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", input, err)
	}

	dir := filepath.Dir(defaultPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(defaultPath, data, 0600); err != nil {
		return "", fmt.Errorf("write credentials: %w", err)
	}

	fmt.Printf("  ✓ Copied to: %s\n", defaultPath)
	return defaultPath, nil
}

// validateCredentials checks that the file is parseable OAuth2 credentials JSON.
func validateCredentials(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Must contain one of the expected top-level keys.
	var raw map[string]interface{}
	if err := parseJSONBytes(data, &raw); err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}
	if _, ok := raw["installed"]; ok {
		return nil // Desktop app credentials
	}
	if _, ok := raw["web"]; ok {
		return fmt.Errorf("this is a 'web' credentials file; mailrat requires a 'Desktop app' OAuth client")
	}
	return fmt.Errorf("unrecognised credentials format (expected 'installed' key for Desktop app)")
}

// ─── formatting helpers ────────────────────────────────────────────────────

func printHeader(title string) {
	fmt.Println()
	fmt.Printf("  %s\n", title)
	fmt.Printf("  %s\n", strings.Repeat("─", len(title)))
	fmt.Println()
}

func printStep(n int, title string) {
	fmt.Println()
	fmt.Printf("  Step %d: %s\n", n, title)
	fmt.Printf("  %s\n", strings.Repeat("·", len(title)+8))
	fmt.Println()
}
