package cli

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
)

func newContactsCmd() *cobra.Command {
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
		RunE: withAppContext(func(ctx context.Context, a *app) error {
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
		}),
	}
	listCmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	listCmd.Flags().BoolVar(&syncFirst, "sync", false, "Sync from Google before listing")
	listCmd.Flags().StringVar(&searchQuery, "search", "", "Filter by name or email")

	// ── add ─────────────────────────────────────────────────────────────────
	var addName, addEmail string

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new contact",
		RunE: withAppContext(func(ctx context.Context, a *app) error {
			aid, err := resolveAccount(a, accountID)
			if err != nil {
				return err
			}

			reader := newPromptReader()
			if addName == "" {
				addName, err = promptLine(reader, "Name: ")
				if err != nil {
					return err
				}
			}
			if addEmail == "" {
				addEmail, err = promptLine(reader, "Email: ")
				if err != nil {
					return err
				}
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
		}),
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
		RunE: withAppContext(func(ctx context.Context, a *app) error {
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

			reader := newPromptReader()
			if !cmd.Flags().Changed("name") {
				editName, err = promptLineDefault(reader, "Name", existing.Name)
				if err != nil {
					return err
				}
			}
			if !cmd.Flags().Changed("email") {
				editEmail, err = promptLineDefault(reader, "Email", existing.Email)
				if err != nil {
					return err
				}
			}

			c, err := svc.Update(ctx, id, editName, editEmail)
			if err != nil {
				return err
			}
			fmt.Printf("✓ Contact updated: %s <%s>\n", c.Name, c.Email)
			return nil
		}),
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
		RunE: withAppContext(func(ctx context.Context, a *app) error {
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
				ok, err := promptConfirm(newPromptReader(), fmt.Sprintf("Delete contact: %s <%s> ? [y/N] ", existing.Name, existing.Email))
				if err != nil {
					return err
				}
				if !ok {
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
		}),
	}
	deleteCmd.Flags().StringVar(&accountID, "account", "", "Account ID")
	deleteCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Skip confirmation prompt")

	contactsCmd.AddCommand(listCmd, addCmd, editCmd, deleteCmd)
	return contactsCmd
}
