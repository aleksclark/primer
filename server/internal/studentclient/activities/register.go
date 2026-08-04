package activities

import "github.com/aleksclark/primer/server/internal/studentclient/contracts"

func init() {
	Register(contracts.KindTerminal, func() Runner { return NewTerminal() })
	Register(contracts.KindTyping, func() Runner { return NewTyping() })
}
