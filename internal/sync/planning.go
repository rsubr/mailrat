package sync

import (
	"fmt"

	"github.com/mailrat/mailrat/internal/storage"
)

type initialSyncPlan struct {
	maxDays       int
	allMailLabels []labelQueryPlan
}

type labelQueryPlan struct {
	config storage.SyncConfig
	query  string
}

type backfillPlan struct {
	config storage.SyncConfig
	query  string
}

func buildInitialSyncPlan(defaultDays int, configs []storage.SyncConfig) initialSyncPlan {
	plan := initialSyncPlan{maxDays: defaultDays}
	for _, cfg := range configs {
		switch {
		case cfg.DaysToSync == 0:
			if query := labelIDToGmailQuery(cfg.LabelID, cfg.LabelName); query != "" {
				plan.allMailLabels = append(plan.allMailLabels, labelQueryPlan{config: cfg, query: query})
			}
		case cfg.DaysToSync > plan.maxDays:
			plan.maxDays = cfg.DaysToSync
		}
	}
	return plan
}

func initialSyncQuery(maxDays int) string {
	return fmt.Sprintf("newer_than:%dd", maxDays)
}

func buildBackfillPlans(configs []storage.SyncConfig) []backfillPlan {
	plans := make([]backfillPlan, 0, len(configs))
	for _, cfg := range configs {
		if cfg.Backfilled {
			continue
		}
		plans = append(plans, backfillPlan{
			config: cfg,
			query:  buildBackfillQuery(cfg),
		})
	}
	return plans
}

func buildBackfillQuery(c storage.SyncConfig) string {
	labelQ := labelIDToGmailQuery(c.LabelID, c.LabelName)
	if labelQ == "" {
		return ""
	}
	if c.DaysToSync == 0 {
		return labelQ
	}
	return fmt.Sprintf("%s newer_than:%dd", labelQ, c.DaysToSync)
}

func attachmentLabelIDs(configs []storage.SyncConfig) []string {
	labelIDs := make([]string, 0, len(configs))
	for _, cfg := range configs {
		if cfg.FetchAttachments {
			labelIDs = append(labelIDs, cfg.LabelID)
		}
	}
	return labelIDs
}

func defaultBackfillConfigs(accountID string) []storage.SyncConfig {
	return []storage.SyncConfig{
		{LabelID: "INBOX", LabelName: "INBOX", AccountID: accountID, DaysToSync: 0, FetchAttachments: true},
		{LabelID: "STARRED", LabelName: "STARRED", AccountID: accountID, DaysToSync: 0, FetchAttachments: true},
	}
}

// labelIDToGmailQuery converts a Gmail label ID to a search query term.
func labelIDToGmailQuery(id, name string) string {
	switch id {
	case "INBOX":
		return "in:inbox"
	case "STARRED":
		return "is:starred"
	case "SENT":
		return "in:sent"
	case "DRAFTS":
		return "in:drafts"
	case "SPAM":
		return "in:spam"
	case "TRASH":
		return "in:trash"
	default:
		if name != "" {
			return "label:" + name
		}
		return ""
	}
}
