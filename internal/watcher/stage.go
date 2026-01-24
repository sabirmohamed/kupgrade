package watcher

import (
	"github.com/sabirmohamed/kupgrade/internal/stage"
)

// NewStageComputer creates a new stage computer
func NewStageComputer(targetVersion string) StageComputer {
	return stage.New(targetVersion)
}
