package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pocketbase/pocketbase/tools/archive"
	"github.com/pocketbase/pocketbase/tools/security"
)

// RestoreBackup restores the backup with the specified name and restarts
// the current running application process.
//
// NB! This feature is experimental and currently is expected to work only on UNIX based systems.
//
// To safely perform the restore it is recommended to have free disk space
// for at least 2x the size of the restored pb_data backup.
//
// The performed steps are:
//
//  1. Download the backup with the specified name in a temp location
//     (this is in case of S3; otherwise it creates a temp copy of the zip)
//
//  2. Extract the backup in a temp directory inside the app "pb_data"
//     (eg. "pb_data/.pb_temp_to_delete/pb_restore").
//
//  3. Move the current app "pb_data" content (excluding the local backups and the special temp dir)
//     under another temp sub dir that will be deleted on the next app start up
//     (eg. "pb_data/.pb_temp_to_delete/old_pb_data").
//     This is because on some environments it may not be allowed
//     to delete the currently open "pb_data" files.
//
//  4. Move the extracted dir content to the app "pb_data".
//
//  5. Restart the app (on successful app bootstrap it will also remove the old pb_data).
//
// If a failure occur during the restore process the dir changes are reverted.
// If for whatever reason the revert is not possible, it panics.
//
// Note that if your pb_data has custom network mounts as subdirectories, then
// it is possible the restore to fail during the `os.Rename` operations
// (see https://github.com/pocketbase/pocketbase/issues/4647).
func (app *BaseApp) RestoreBackup(ctx context.Context, name string) error {
	if app.Store().Has(StoreKeyActiveBackup) {
		return errors.New("try again later - another backup/restore operation has already been started")
	}

	app.Store().Set(StoreKeyActiveBackup, name)
	defer app.Store().Remove(StoreKeyActiveBackup)

	event := new(BackupEvent)
	event.App = app
	event.Context = ctx
	event.Name = name
	// default root dir entries to exclude from the backup restore
	event.Exclude = []string{LocalBackupsDirName, LocalTempDirName, LocalAutocertCacheDirName, lostFoundDirName}

	return app.OnBackupRestore().Trigger(event, func(e *BackupEvent) error {
		if runtime.GOOS == "windows" {
			return errors.New("restore is not supported on Windows")
		}

		// make sure that the special temp directory exists
		// note: it needs to be inside the current pb_data to avoid "cross-device link" errors
		localTempDir := filepath.Join(e.App.DataDir(), LocalTempDirName)
		if err := os.MkdirAll(localTempDir, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create a temp dir: %w", err)
		}

		fsys, err := e.App.NewBackupsFilesystem()
		if err != nil {
			return err
		}
		defer fsys.Close()

		fsys.SetContext(e.Context)

		if ok, _ := fsys.Exists(name); !ok {
			return fmt.Errorf("missing or invalid backup file %q to restore", name)
		}

		extractedDataDir := filepath.Join(localTempDir, "pb_restore_"+security.PseudorandomString(8))
		defer os.RemoveAll(extractedDataDir)

		// extract the zip
		if e.App.Settings().Backups.S3.Enabled {
			br, err := fsys.GetReader(name)
			if err != nil {
				return err
			}
			defer br.Close()

			// create a temp zip file from the blob.Reader and try to extract it
			tempZip, err := os.CreateTemp(localTempDir, "pb_restore_zip")
			if err != nil {
				return err
			}
			defer os.Remove(tempZip.Name())
			defer tempZip.Close() // note: this technically shouldn't be necessary but it is here to workaround platforms discrepancies

			_, err = io.Copy(tempZip, br)
			if err != nil {
				return err
			}

			err = archive.Extract(tempZip.Name(), extractedDataDir)
			if err != nil {
				return err
			}

			// remove the temp zip file since we no longer need it
			// (this is in case the app restarts and the defer calls are not called)
			_ = tempZip.Close()
			err = os.Remove(tempZip.Name())
			if err != nil {
				e.App.Logger().Warn(
					"[RestoreBackup] Failed to remove the temp zip backup file",
					slog.String("file", tempZip.Name()),
					slog.String("error", err.Error()),
				)
			}
		} else {
			// manually construct the local path to avoid creating a copy of the zip file
			// since the blob reader currently doesn't implement ReaderAt
			zipPath := filepath.Join(e.App.DataDir(), LocalBackupsDirName, filepath.Base(name))

			err = archive.Extract(zipPath, extractedDataDir)
			if err != nil {
				return err
			}
		}

		// PostgreSQL backups contain pg_dump archives. Restoring two independent
		// databases while the application is running is not atomic, so it must be
		// performed as a controlled maintenance operation with pg_restore.
		if _, err := os.Stat(filepath.Join(extractedDataDir, postgresDataDumpFilename)); err == nil {
			return errors.New("PostgreSQL backup restore is not supported from the dashboard; restore data.dump and auxiliary.dump with pg_restore during maintenance")
		}

		return errors.New("invalid backup archive: missing data.dump; PostgreSQL backups must be restored with pg_restore during maintenance")
	})
}
