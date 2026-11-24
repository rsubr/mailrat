package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mailrat/mailrat/internal/storage"
)

// TestG1AccountsManagerRace hammers the swap of the OAuth manager (credential
// import) against every reader. Under -race it failed while a.accounts was
// accessed without a lock.
func TestG1AccountsManagerRace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MAILRAT_DATA_DIR", dir)
	a := appWithStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.ctx = ctx
	a.SetEventHandler(func(string, any) {})
	creds := filepath.Join(t.TempDir(), "c.json")
	body := `{"installed":{"client_id":"i","client_secret":"s","auth_uri":"http://127.0.0.1:1/auth","token_uri":"http://127.0.0.1:1/token","redirect_uris":["http://localhost"]}}`
	if err := os.WriteFile(creds, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	run := func(f func(i int)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				f(i)
			}
		}()
	}
	run(func(int) { _ = a.ImportCredentials(creds) })
	run(func(int) { _ = a.HasCredentials() })
	run(func(int) { _ = a.Sync("unified") })
	run(func(int) { _, _ = a.GetAccounts() })
	run(func(i int) { _, _ = a.CreateLabel("nope", "x") })
	run(func(i int) {
		id := fmt.Sprintf("r%d", i)
		_ = a.store.UpsertAccount(storageAccount(id))
		_ = a.RemoveAccount(id)
	})
	wg.Wait()
}

func storageAccount(id string) storage.Account {
	return storage.Account{ID: id, Email: id + "@x.test"}
}
