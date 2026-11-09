package github

import (
	"strconv"
	"time"
)

// Event keys are durable identities shared by webhooks and reconciliation.
func reviewEventKey(reviewID int64) string {
	return "review:" + strconv.FormatInt(reviewID, 10)
}

func mergedEventKey(repositoryID, resourceID string) string {
	return "merged:" + repositoryID + ":" + resourceID
}

func discoveredEventKey(repositoryID, resourceID, headSHA string, updatedAt time.Time) string {
	return "discovered:" + repositoryID + ":" + resourceID + ":" + headSHA + ":" + updatedAt.Format(time.RFC3339Nano)
}
