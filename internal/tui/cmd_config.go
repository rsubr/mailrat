package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mailrat/mailrat/internal/mail"
	"github.com/mailrat/mailrat/internal/storage"
)

// loadConfigCmd loads labels and existing sync configs for the given account,
// merging them with smart defaults (INBOX/STARRED: all+attachments, rest: 30d).
func loadConfigCmd(store *storage.Store, acctID string) tea.Cmd {
	return func() tea.Msg {
		labels, err := store.ListLabels(acctID)
		if err != nil {
			return msgConfigLoaded{err: err}
		}

		existing, _ := store.ListSyncConfigs(acctID)
		cfgMap := make(map[string]storage.SyncConfig, len(existing))
		for _, cfg := range existing {
			cfgMap[cfg.LabelID] = cfg
		}

		rows := make([]configRow, 0, len(labels))
		for _, label := range labels {
			rows = append(rows, configRowForLabel(label, cfgMap))
		}

		return msgConfigLoaded{rows: rows}
	}
}

func configRowForLabel(label storage.Label, cfgMap map[string]storage.SyncConfig) configRow {
	row := configRow{
		LabelID:          label.ID,
		LabelName:        label.Name,
		DaysToSync:       30,
		FetchAttachments: false,
	}

	if cfg, ok := cfgMap[label.ID]; ok {
		row.DaysToSync = cfg.DaysToSync
		row.FetchAttachments = cfg.FetchAttachments
		return row
	}

	if label.ID == mail.LabelInbox || label.ID == mail.LabelStarred {
		row.DaysToSync = 0
		row.FetchAttachments = true
	}

	return row
}

// saveConfigCmd persists all config rows to the database.
func saveConfigCmd(store *storage.Store, acctID string, rows []configRow) tea.Cmd {
	return func() tea.Msg {
		for _, row := range rows {
			if err := store.UpsertSyncConfig(storage.SyncConfig{
				LabelID:          row.LabelID,
				AccountID:        acctID,
				LabelName:        row.LabelName,
				DaysToSync:       row.DaysToSync,
				FetchAttachments: row.FetchAttachments,
			}); err != nil {
				return msgConfigSaved{err: err}
			}
		}
		return msgConfigSaved{}
	}
}
