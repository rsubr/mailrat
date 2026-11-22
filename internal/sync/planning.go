package sync

import (
	"fmt"
	"strings"

	"github.com/mailrat/mailrat/internal/storage"
)

type labelQueryPlan struct {
	config storage.SyncConfig
	query  string
	limit  int64 // 0 = unlimited
}

type backfillPlan = labelQueryPlan

// scopeQuery turns a label's sync scope into a Gmail query and a result cap:
// everything, last N days (newer_than), or the newest N emails (limit).
func scopeQuery(c storage.SyncConfig) (query string, limit int64) {
	labelQ := labelIDToGmailQuery(c.LabelID, c.LabelName)
	switch {
	case labelQ == "":
		return "", 0
	case c.MaxEmails > 0:
		return labelQ, int64(c.MaxEmails)
	case c.DaysToSync == 0:
		return labelQ, 0
	default:
		return fmt.Sprintf("%s newer_than:%dd", labelQ, c.DaysToSync), 0
	}
}

// buildLabelPlans returns one plan per config; labels whose query cannot be
// built are dropped.
func buildLabelPlans(configs []storage.SyncConfig, onlyPending bool) []labelQueryPlan {
	plans := make([]labelQueryPlan, 0, len(configs))
	for _, cfg := range configs {
		if onlyPending && cfg.Backfilled {
			continue
		}
		q, limit := scopeQuery(cfg)
		plans = append(plans, labelQueryPlan{config: cfg, query: q, limit: limit})
	}
	return plans
}

func initialSyncQuery(maxDays int) string {
	if maxDays <= 0 {
		return ""
	}
	return fmt.Sprintf("newer_than:%dd", maxDays)
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
	case "DRAFT", "DRAFTS":
		return "in:drafts"
	case "IMPORTANT":
		return "is:important"
	case "SPAM":
		return "in:spam"
	case "TRASH":
		return "in:trash"
	default:
		if strings.ContainsAny(name, " \t") {
			return `label:"` + name + `"`
		}
		if name != "" {
			return "label:" + name
		}
		return ""
	}
}
