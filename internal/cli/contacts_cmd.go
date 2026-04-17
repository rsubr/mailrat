package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mailrat/mailrat/internal/storage"
	"github.com/spf13/cobra"
)

func init() {
	var accountID string

	contactsCmd := &cobra.Command{
		Use:   "contacts",
		Short: "Manage contacts",
	}

	// ── list ────────────────────────────────────────────────────────────────
	var syncFirst bool
	var searchQuery string

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			ctx := context.Background()
			aid, err := resolveAccount(a, accountID)
			if err != nil {
				return err
			}

			svc, err := a.contactsService(ctx, aid)
			if err != nil {
				return err
			}

			if syncFirst {
				if err := svc.Sync(ctx); err != nil {
					return err
				}
			}

			if searchQuery != "" {
				results, err := svc.Search(searchQuery, 50)
				if err != nil {
					return err
				}
				printContacts(results)
			} else {
				results, err := svc.List()
				if err != nil {
					return err
				}
				if len(results) == 0 {
					fmt.Println("No contacts. Run: mailrat contacts list --sync")
					return nil
				}
				printContacts(results)
			}
			return nil
		},
	}
	listCmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	listCmd.Flags().BoolVar(&syncFirst, "sync", false, "Sync from Google before listing")
	listCmd.Flags().StringVar(&searchQuery, "search", "", "Filter by name or email")

	// ── add ─────────────────────────────────────────────────────────────────
	var addName, addEmail string

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new contact",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			ctx := context.Background()
			aid, err := resolveAccount(a, accountID)
			if err != nil {
				return err
			}

			reader := bufio.NewReader(os.Stdin)
			if addName == "" {
				fmt.Print("Name: ")
				addName, _ = reader.ReadString('\n')
				addName = strings.TrimSpace(addName)
			}
			if addEmail == "" {
				fmt.Print("Email: ")
				addEmail, _ = reader.ReadString('\n')
				addEmail = strings.TrimSpace(addEmail)
			}
			if addEmail == "" {
				return fmt.Errorf("email is required")
			}

			svc, err := a.contactsService(ctx, aid)
			if err != nil {
				return err
			}

			c, err := svc.Create(ctx, addName, addEmail)
			if err != nil {
				return err
			}
			fmt.Printf("✓ Contact created: %s <%s>  (id: %s)\n", c.Name, c.Email, c.ID)
			return nil
		},
	}
	addCmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	addCmd.Flags().StringVar(&addName, "name", "", "Contact name")
	addCmd.Flags().StringVar(&addEmail, "email", "", "Contact email")

	// ── edit ────────────────────────────────────────────────────────────────
	var editName, editEmail string

	editCmd := &cobra.Command{
		Use:   "edit <contact-id>",
		Short: "Edit a contact's name and/or email",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			ctx := context.Background()
			aid, err := resolveAccount(a, accountID)
			if err != nil {
				return err
			}

			svc, err := a.contactsService(ctx, aid)
			if err != nil {
				return err
			}

			id := args[0]

			// Load existing values to show as defaults.
			existing, err := a.store.GetContact(aid, id)
			if err != nil {
				return fmt.Errorf("contact %q not found; run: mailrat contacts list --sync", id)
			}

			reader := bufio.NewReader(os.Stdin)
			if !cmd.Flags().Changed("name") {
				fmt.Printf("Name [%s]: ", existing.Name)
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(input)
				if input != "" {
					editName = input
				} else {
					editName = existing.Name
				}
			}
			if !cmd.Flags().Changed("email") {
				fmt.Printf("Email [%s]: ", existing.Email)
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(input)
				if input != "" {
					editEmail = input
				} else {
					editEmail = existing.Email
				}
			}

			c, err := svc.Update(ctx, id, editName, editEmail)
			if err != nil {
				return err
			}
			fmt.Printf("✓ Contact updated: %s <%s>\n", c.Name, c.Email)
			return nil
		},
	}
	editCmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	editCmd.Flags().StringVar(&editName, "name", "", "New name")
	editCmd.Flags().StringVar(&editEmail, "email", "", "New email")

	// ── delete ───────────────────────────────────────────────────────────────
	var forceDelete bool

	deleteCmd := &cobra.Command{
		Use:   "delete <contact-id>",
		Short: "Delete a contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			defer a.close()

			ctx := context.Background()
			aid, err := resolveAccount(a, accountID)
			if err != nil {
				return err
			}

			id := args[0]

			existing, err := a.store.GetContact(aid, id)
			if err != nil {
				return fmt.Errorf("contact %q not found; run: mailrat contacts list --sync", id)
			}

			if !forceDelete {
				fmt.Printf("Delete contact: %s <%s> ? [y/N] ", existing.Name, existing.Email)
				reader := bufio.NewReader(os.Stdin)
				input, _ := reader.ReadString('\n')
				if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(input)), "y") {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			svc, err := a.contactsService(ctx, aid)
			if err != nil {
				return err
			}

			if err := svc.Delete(ctx, id); err != nil {
				return err
			}
			fmt.Printf("✓ Contact deleted: %s <%s>\n", existing.Name, existing.Email)
			return nil
		},
	}
	deleteCmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	deleteCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Skip confirmation prompt")

	contactsCmd.AddCommand(listCmd, addCmd, editCmd, deleteCmd)
	rootCmd.AddCommand(contactsCmd)
}

// resolveAccount returns accountID if set, otherwise the first stored account.
func resolveAccount(a *app, accountID string) (string, error) {
	if accountID != "" {
		return accountID, nil
	}
	accs, err := a.accounts.ListAccounts()
	if err != nil {
		return "", err
	}
	if len(accs) == 0 {
		return "", fmt.Errorf("no accounts configured; run: mailrat accounts add")
	}
	return accs[0].ID, nil
}

func printContacts(contacts []storage.Contact) {
	fmt.Printf("%-35s  %-30s  %s\n", "NAME", "EMAIL", "ID")
	fmt.Println(strings.Repeat("─", 100))
	for _, c := range contacts {
		fmt.Printf("%-35s  %-30s  %s\n",
			truncate(c.Name, 35),
			truncate(c.Email, 30),
			c.ID,
		)
	}
}
