package commands

import (
	"lime/backend"
)

type (
	UndoCommand struct {
		backend.DefaultCommand
		backend.BypassUndoCommand
		hard bool
	}
	RedoCommand struct {
		backend.DefaultCommand
		backend.BypassUndoCommand
		hard bool
	}
)

func (c *UndoCommand) Run(v *backend.View, e *backend.Edit, args backend.Args) error {
	v.UndoStack().Undo(c.hard)
	return nil
}

func (c *RedoCommand) Run(v *backend.View, e *backend.Edit, args backend.Args) error {
	v.UndoStack().Redo(c.hard)
	return nil
}

func init() {
	register([]cmd{
		{"undo", &UndoCommand{hard: true}},
		{"redo", &RedoCommand{hard: true}},
		{"soft_undo", &UndoCommand{}},
		{"soft_redo", &RedoCommand{}},
	})
}
