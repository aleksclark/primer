package activities

import "github.com/aleksclark/primer/server/internal/studentclient/contracts"

func checkTreeEmpty(t contracts.CheckTree) bool {
	return t.CheckID == "" && len(t.All) == 0 && len(t.Any) == 0
}
