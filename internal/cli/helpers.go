package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func withApp(run func(*app) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return runWithApp(run)
	}
}

func runWithApp(run func(*app) error) error {
	if run == nil {
		return nil
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.close()
	return run(a)
}

func withAppContext(run func(context.Context, *app) error) func(*cobra.Command, []string) error {
	return withApp(func(a *app) error {
		return run(context.Background(), a)
	})
}

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

func resolveAccountEmail(a *app, accountID string) (string, error) {
	accs, err := a.accounts.ListAccounts()
	if err != nil {
		return "", err
	}
	for _, acc := range accs {
		if acc.ID == accountID {
			return acc.Email, nil
		}
	}
	return "", fmt.Errorf("account %q not found", accountID)
}

func resolveMessageAccount(a *app, messageID string) (string, error) {
	accs, err := a.accounts.ListAccounts()
	if err != nil {
		return "", err
	}
	for _, acc := range accs {
		if _, err := a.store.GetMessage(acc.ID, messageID); err == nil {
			return acc.ID, nil
		}
	}
	return "", fmt.Errorf("message %q not found in any account", messageID)
}

func newPromptReader() *bufio.Reader {
	return bufio.NewReader(os.Stdin)
}

func promptLine(reader *bufio.Reader, label string) (string, error) {
	fmt.Print(label)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

func promptLineDefault(reader *bufio.Reader, label, current string) (string, error) {
	input, err := promptLine(reader, fmt.Sprintf("%s [%s]: ", label, current))
	if err != nil {
		return "", err
	}
	if input == "" {
		return current, nil
	}
	return input, nil
}

func promptConfirm(reader *bufio.Reader, prompt string) (bool, error) {
	input, err := promptLine(reader, prompt)
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(input)), "y"), nil
}

func promptExactMatch(reader *bufio.Reader, prompt, expected string) (bool, error) {
	input, err := promptLine(reader, prompt)
	if err != nil {
		return false, err
	}
	return input == expected, nil
}

func promptMultilineBody(reader *bufio.Reader, header string) (string, error) {
	fmt.Println(header)
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "." {
			return strings.Join(lines, "\n"), nil
		}
		lines = append(lines, line)
	}
}
