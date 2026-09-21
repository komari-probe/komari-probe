// Package backup creates downloadable backup archives and stages uploaded
// archives for restoration on the next process startup.
package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/komari-monitor/komari/internal/platform/dbcore"
)

// copyFile 复制单个文件到目标路径（会确保父目录存在）
func copyFile(srcPath, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %v", err)
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer src.Close()

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer dest.Close()

	if _, err = io.Copy(dest, src); err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}
	return nil
}

// walkDirToZip 将 contentDir 的内容写入 zip writer。
// 注意：contentDir 不应包含被 walk 的 zip 文件本身。
func walkDirToZip(zipWriter *zip.Writer, contentDir string) error {
	return filepath.Walk(contentDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(contentDir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		zipPath := filepath.ToSlash(rel)
		if info.IsDir() {
			_, err := zipWriter.CreateHeader(&zip.FileHeader{
				Name:     zipPath + "/",
				Method:   zip.Deflate,
				Modified: info.ModTime(),
			})
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		w, err := zipWriter.CreateHeader(&zip.FileHeader{
			Name:     zipPath,
			Method:   zip.Deflate,
			Modified: info.ModTime(),
		})
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		return err
	})
}

// writeBackupMarkup 追加备份标记文件到 zip。
func writeBackupMarkup(zipWriter *zip.Writer) error {
	now := time.Now().UTC()
	markupContent := "此文件为 Komari 备份标记文件，请勿删除。\nThis is a Komari backup markup file, please do not delete.\n\n备份时间 / Backup Time: " + now.Format(time.RFC3339Nano)
	markupWriter, err := zipWriter.CreateHeader(&zip.FileHeader{
		Name:     "komari-backup-markup",
		Method:   zip.Deflate,
		Modified: now,
	})
	if err != nil {
		return err
	}
	_, err = markupWriter.Write([]byte(markupContent))
	return err
}

// backupSQLiteTo 使用 SQLite VACUUM INTO 将当前数据库一致性备份到指定路径。
// destDBPath 会先删除以防 SQLite 报 "output file already exists"。
func backupSQLiteTo(destDBPath string) error {
	if err := os.MkdirAll(filepath.Dir(destDBPath), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory for db: %v", err)
	}

	// 确保目标文件不存在（VACUUM INTO 要求目标文件不存在）
	_ = os.Remove(destDBPath)

	db := dbcore.GetDBInstance()
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying database connection: %v", err)
	}

	// Windows 下 VACUUM INTO 传绝对路径时，统一使用正斜杠避免路径解析歧义
	safePath := filepath.ToSlash(destDBPath)
	safePath = strings.ReplaceAll(safePath, "'", "''")
	vacuumSQL := fmt.Sprintf("VACUUM INTO '%s'", safePath)
	if _, err = sqlDB.Exec(vacuumSQL); err != nil {
		return fmt.Errorf("sqlite VACUUM INTO failed: %v", err)
	}
	return nil
}
