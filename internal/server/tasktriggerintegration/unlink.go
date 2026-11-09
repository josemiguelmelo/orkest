package tasktriggerintegration

type UnlinkSnapshot struct {
	Integration   Integration
	Link          ResourceLink
	CurrentTaskID string
	Suppressed    bool
}
