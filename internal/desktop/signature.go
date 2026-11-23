package desktop

import (
	"fmt"
	"strings"

	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/mail"
	"github.com/mailrat/mailrat/internal/storage"
)

// refreshSendAs re-fetches the account's send-as aliases into the local cache.
func (a *App) refreshSendAs(acc storage.Account, client *gmail.Client) {
	list, err := client.ListSendAs(a.ctx)
	if err != nil {
		a.logWarning(fmt.Sprintf("send-as %s: %v", acc.Email, err))
		return
	}
	rows := make([]storage.SendAs, len(list))
	for i, s := range list {
		rows[i] = storage.SendAs{Email: s.Email, DisplayName: s.DisplayName,
			SignatureHTML: s.SignatureHTML, IsPrimary: s.IsPrimary, IsDefault: s.IsDefault}
	}
	if err := a.store.ReplaceSendAs(acc.ID, rows); err != nil {
		a.logWarning(fmt.Sprintf("store send-as %s: %v", acc.Email, err))
	}
}

// Signature returns the sanitized signature HTML for fromEmail, falling back
// to the account's default alias, then its primary. Empty if none is cached.
func (a *App) Signature(accountID, fromEmail string) string {
	list, err := a.store.ListSendAs(accountID)
	if err != nil {
		return ""
	}
	return mail.SanitizeHTML(pickSignature(list, fromEmail))
}

func pickSignature(list []storage.SendAs, fromEmail string) string {
	var def, primary *storage.SendAs
	for i := range list {
		s := &list[i]
		if fromEmail != "" && strings.EqualFold(s.Email, fromEmail) {
			return s.SignatureHTML
		}
		if s.IsDefault {
			def = s
		}
		if s.IsPrimary {
			primary = s
		}
	}
	if def != nil {
		return def.SignatureHTML
	}
	if primary != nil {
		return primary.SignatureHTML
	}
	return ""
}
