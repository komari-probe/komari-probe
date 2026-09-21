package router

import (
	"path/filepath"

	"github.com/komari-monitor/komari/internal/features/backup"
	"github.com/komari-monitor/komari/internal/features/plugin"
	"github.com/komari-monitor/komari/internal/features/theme"
	"github.com/komari-monitor/komari/internal/platform/upload"
)

func NewArchiveUploadHandler() *upload.Handler {
	return upload.NewHandler(upload.DefaultStore, map[upload.Purpose]upload.Finalizer{
		upload.PurposeBackup: finalizeBackupUpload,
		upload.PurposePlugin: finalizePluginUpload,
		upload.PurposeTheme:  finalizeThemeUpload,
	})
}

func finalizeBackupUpload(session upload.Session) (upload.Result, error) {
	if err := backup.FinalizeUploadedRestore(session.ArchivePath, session.Metadata.Filename); err != nil {
		return upload.Result{}, err
	}
	return upload.Result{
		Message: "Backup uploaded successfully. The service will restart and apply the backup.",
		Data:    map[string]string{"path": filepath.Join(".", "data", "backup.zip")},
	}, nil
}

func finalizePluginUpload(session upload.Session) (upload.Result, error) {
	info, err := plugin.InstallZip(session.ArchivePath)
	if err != nil {
		return upload.Result{}, err
	}
	return upload.Result{Message: "Plugin uploaded successfully.", Data: info}, nil
}

func finalizeThemeUpload(session upload.Session) (upload.Result, error) {
	info, err := theme.InstallZip(session.ArchivePath)
	if err != nil {
		return upload.Result{}, err
	}
	return upload.Result{Message: "Theme uploaded successfully.", Data: info}, nil
}
