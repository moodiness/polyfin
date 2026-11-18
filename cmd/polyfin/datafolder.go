package main

import (
	"log/slog"

	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/discovery"
)

// prepareDataFolder makes the data folder of cfg, or looks at the one
// that is there, before serve writes into it (see
// config.PrepareDataFolder). It warns of two things System › Health tells
// too: a folder that was chosen and that another account of the system
// can change, which it returns, and a folder that is the default one of
// earlier versions, in the system's temporary folder, kept because it is
// there.
func prepareDataFolder(cfg config.Config, logger *slog.Logger) (*config.DataFolderFault, error) {
	fault, err := config.PrepareDataFolder(cfg.DataDir, discovery.InContainer())
	if err != nil {
		return nil, err
	}
	if fault != nil {
		logger.Warn("Another account of this system can change the files Polyfin keeps and runs in its data folder: the folder "+fault.String(),
			"folder", fault.Dir, "mend", fault.Command())
	}
	if cfg.MoveDataDirTo != "" {
		logger.Warn("Polyfin keeps its files in the default data folder of earlier versions, in the system's temporary folder, which the system may empty: "+
			"stop Polyfin and move the folder, or set POLYFIN_DATA_DIR", "folder", cfg.DataDir, "move to", cfg.MoveDataDirTo)
	}
	return fault, nil
}
