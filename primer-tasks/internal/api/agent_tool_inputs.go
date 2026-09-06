package api

// Fantasy's public schema reflector does not flatten embedded structs or model
// time.Time as an RFC3339 string. Keep this model-facing input flat and convert
// through the same parent domain types at the confirmed effect boundary.
type agentScheduleEditInput struct {
	ScheduleID       string `json:"scheduleId"`
	ExpectedVersion  int    `json:"expectedVersion,omitempty"`
	StudentID        string `json:"studentId"`
	TemplateID       string `json:"templateId"`
	RevisionID       string `json:"revisionId"`
	Kind             string `json:"kind"`
	Timezone         string `json:"timezone"`
	StartAt          string `json:"startAt"`
	EndAt            string `json:"endAt,omitempty"`
	RRULE            string `json:"rrule"`
	DueOffsetMinutes int    `json:"dueOffsetMinutes"`
}
