package api

import (
	"charm.land/fantasy"
	"encoding/json"
	"primer-tasks/internal/domain/parent"
	"time"
)

func scriptedCreatePreview(call fantasy.Call) string {
	result := toolResultJSON(call, "list_students")
	items, _ := result["items"].([]any)
	if len(items) != 1 {
		return `{"kind":"create_task_schedule","targetIds":[]}`
	}
	student, _ := items[0].(map[string]any)
	id, _ := student["id"].(string)
	data, _ := json.Marshal(map[string]any{"summary": "Prepare task and schedule", "kind": parent.ActionCreateTaskSchedule, "targetIds": []string{id}, "payload": map[string]any{"StudentID": id, "Title": "Scripted parent task", "Instructions": "Complete the task, then ask a parent to check it.", "StartAt": time.Now().UTC().Add(24 * time.Hour)}})
	return string(data)
}
