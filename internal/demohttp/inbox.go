package demohttp

import (
	"net/http"

	"plenilune/internal/analysis"
	"plenilune/internal/evidence"
	"plenilune/internal/inbox"
)

// verdictInbox is the server's trigger store (P2-8): pinned object
// watches, the last recorded outcome per object and the fired inbox
// entries. Like compareCache it lives in server-process memory only —
// a restart starts from an empty inbox and no watches.
var verdictInbox = inbox.NewStore()

type watchesDocument struct {
	Watches []inbox.Watch `json:"watches"`
}

type inboxDocument struct {
	Entries []inbox.Entry `json:"entries"`
	Unread  int           `json:"unread"`
}

// observeAnalysis feeds every engine-decided object of a completed
// analysis to the trigger store. Structural objects (added, removed,
// unchanged, unsupported, not analyzed) carry no engine verdict and
// are not observations. The analysis flow evaluates no obligations,
// so obligation state is left untouched by these observations.
func observeAnalysis(result analysis.Result) {
	for _, obj := range result.Objects {
		var verdict string
		switch {
		case obj.ACL != nil:
			verdict = obj.ACL.Verdict
		case obj.PrefixList != nil:
			verdict = obj.PrefixList.Report.Verdict
		default:
			continue
		}
		verdictInbox.Observe(inbox.Observation{
			Domain:        obj.Domain,
			Name:          obj.Name,
			Verdict:       verdict,
			ArtifactKind:  inbox.ArtifactAnalysis,
			ContentSHA256: result.ContentSHA256,
			SHA256:        result.SHA256,
		})
	}
}

// observeComparison feeds the single ACL of a completed comparison to
// the trigger store, with its obligation outcomes: this is the only
// flow that can fire the obligation_failed condition.
func observeComparison(c evidence.Comparison) {
	name := c.After.Name
	if name == "" {
		name = c.Before.Name
	}
	if name == "" {
		return
	}
	var violated []string
	for _, ob := range c.Report.Obligations {
		if ob.Status == "VIOLATED" {
			violated = append(violated, ob.ID)
		}
	}
	verdictInbox.Observe(inbox.Observation{
		Domain:               inbox.DomainACL,
		Name:                 name,
		Verdict:              c.Report.Verdict,
		ObligationsEvaluated: true,
		ViolatedObligations:  violated,
		ArtifactKind:         inbox.ArtifactComparison,
		ContentSHA256:        c.ContentSHA256,
		SHA256:               c.SHA256,
	})
}

func handleListWatches(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, watchesDocument{Watches: verdictInbox.Watches()})
}

func handleReplaceWatches(w http.ResponseWriter, r *http.Request) {
	var in watchesDocument
	if !decodeRequest(w, r, maxParseBody, &in) {
		return
	}
	if err := verdictInbox.SetWatches(in.Watches); err != nil {
		writeError(w, http.StatusBadRequest, apiError{Code: "WATCHES_INVALID", Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, watchesDocument{Watches: verdictInbox.Watches()})
}

func handleListInbox(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, inboxDocument{Entries: verdictInbox.Entries(), Unread: verdictInbox.Unread()})
}

type inboxReadRequest struct {
	ID  int64 `json:"id"`
	All bool  `json:"all"`
}

func handleInboxRead(w http.ResponseWriter, r *http.Request) {
	var in inboxReadRequest
	if !decodeRequest(w, r, maxParseBody, &in) {
		return
	}
	if in.All {
		verdictInbox.MarkAllRead()
	} else if !verdictInbox.MarkRead(in.ID) {
		writeError(w, http.StatusBadRequest, apiError{Code: "INBOX_INVALID", Message: "unknown inbox entry id", Field: "id"})
		return
	}
	writeJSON(w, http.StatusOK, inboxDocument{Entries: verdictInbox.Entries(), Unread: verdictInbox.Unread()})
}
