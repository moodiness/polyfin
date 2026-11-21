package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/tasks"
)

// subtitleTracksTask deletes the subtitle tracks read whole that no
// playback used for playback.TrackKeepDays (see
// playback.Service.SweepSubtitleTracks), at start and every day, and logs
// how many it deleted.
func subtitleTracksTask(player *playback.Service, logger *slog.Logger) tasks.Task {
	return tasks.Task{
		Key:      "CleanSubtitleTracks",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en":    {Name: "Clean the kept subtitle tracks", Description: "Deletes the subtitle tracks read from inside files that no playback used for 60 days, unless their version was analysed in that time. A later playback reads them again through the file's index."},
			"fr":    {Name: "Nettoyer les pistes de sous-titres gardées", Description: "Supprime les pistes de sous-titres lues dans les fichiers qu’aucune lecture n’a utilisées depuis 60 jours, sauf si leur version a été analysée depuis. Une lecture ultérieure les relit grâce à l’index du fichier."},
			"de":    {Name: "Behaltene Untertitelspuren bereinigen", Description: "Löscht die aus Dateien gelesenen Untertitelspuren, die seit 60 Tagen keine Wiedergabe verwendet hat, außer ihre Version wurde seitdem analysiert. Eine spätere Wiedergabe liest sie erneut über den Index der Datei."},
			"es":    {Name: "Limpiar las pistas de subtítulos conservadas", Description: "Borra las pistas de subtítulos leídas dentro de los archivos que ninguna reproducción ha usado en 60 días, salvo si su versión se analizó en ese tiempo. Una reproducción posterior las vuelve a leer mediante el índice del archivo."},
			"it":    {Name: "Pulisci le tracce di sottotitoli conservate", Description: "Elimina le tracce di sottotitoli lette all’interno dei file che nessuna riproduzione ha usato da 60 giorni, a meno che la loro versione sia stata analizzata nel frattempo. Una riproduzione successiva le rilegge tramite l’indice del file."},
			"pt-BR": {Name: "Limpar as faixas de legendas mantidas", Description: "Exclui as faixas de legendas lidas dentro dos arquivos que nenhuma reprodução usou em 60 dias, a menos que sua versão tenha sido analisada nesse tempo. Uma reprodução posterior as lê de novo pelo índice do arquivo."},
		},
		Triggers: []tasks.Trigger{tasks.AtStartup(), tasks.Every(playback.TrackSweepInterval)},
		Limits:   tasks.Limits{MinInterval: time.Hour},
		Run: func(ctx context.Context) error {
			deleted, err := player.SweepSubtitleTracks(ctx)
			if deleted > 0 {
				logger.Info("Deleted the subtitle tracks unused for long", "tracks", deleted)
			}
			return err
		},
	}
}
