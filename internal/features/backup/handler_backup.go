package backup

import (
	"archive/zip"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sonar-probe/sonar/internal/platform/dbcore"
	"github.com/sonar-probe/sonar/internal/platform/respond"
)

// DownloadBackup 使用白名单打包 ./data 及数据库文件为 zip 并下载，
// 同时归档到 ./data/backup/ 确保 Docker 挂载后备份文件可持久化。
//
// 归档文件由前端后续统一管理，服务端只负责生成并保存。
func DownloadBackup(c *gin.Context) {
	backupDir := filepath.Join(".", "data", "backup")

	// 1) 创建临时目录，内容隔离到 content/ 子目录
	tempDir, err := os.MkdirTemp("", "komari-backup-*")
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error creating temporary directory: %v", err))
		return
	}
	defer os.RemoveAll(tempDir)

	contentDir := filepath.Join(tempDir, "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error creating content directory: %v", err))
		return
	}

	// 2) 复制白名单文件到 content 目录
	if err := copyWhitelistedFiles(contentDir); err != nil {
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error copying data to temp: %v", err))
		return
	}

	// 3) 处理数据库备份 -> content/komari.db
	destDB := filepath.Join(contentDir, "komari.db")
	dbFilePath := dbcore.DatabaseFile

	if dbcore.IsSQLite() {
		if err := backupSQLiteTo(destDB); err != nil {
			respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error backing up sqlite database: %v", err))
			return
		}
	} else if dbFilePath != "" {
		if _, err := os.Stat(dbFilePath); err == nil {
			if err := copyFile(dbFilePath, destDB); err != nil {
				respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error copying database file: %v", err))
				return
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error stating database file: %v", err))
			return
		}
	}

	// 4) 打包到临时 ZIP（放在 tempDir 下，与 content 平级）
	tempZipPath := filepath.Join(tempDir, "output.zip")
	tempZip, err := os.Create(tempZipPath)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error creating temp zip: %v", err))
		return
	}
	zipWriter := zip.NewWriter(tempZip)

	// 只 walk content 目录，避免 output.zip 被打包进去
	if err := walkDirToZip(zipWriter, contentDir); err != nil {
		zipWriter.Close()
		tempZip.Close()
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error archiving temp folder: %v", err))
		return
	}

	if err := writeBackupMarkup(zipWriter); err != nil {
		zipWriter.Close()
		tempZip.Close()
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error writing backup markup: %v", err))
		return
	}

	if err := zipWriter.Close(); err != nil {
		tempZip.Close()
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error finalizing zip: %v", err))
		return
	}
	tempZip.Close()

	// 5) 归档到 data/backup/。先写临时文件再原子发布。
	ts := time.Now().UTC().Format("20060102-150405.000000")
	archiveName := fmt.Sprintf("backup-%s.zip", ts)

	archivePath := filepath.Join(backupDir, archiveName)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error creating backup directory: %v", err))
		return
	}

	archiveTemp, err := os.CreateTemp(backupDir, ".backup-*.tmp")
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error creating archive temp file: %v", err))
		return
	}
	archiveTempPath := archiveTemp.Name()
	if err := archiveTemp.Close(); err != nil {
		os.Remove(archiveTempPath)
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error closing archive temp file: %v", err))
		return
	}
	defer os.Remove(archiveTempPath)
	if err := copyFile(tempZipPath, archiveTempPath); err != nil {
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error archiving backup: %v", err))
		return
	}
	if err := os.Rename(archiveTempPath, archivePath); err != nil {
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error publishing backup archive: %v", err))
		return
	}

	// 6) 发送给客户端
	c.Writer.Header().Set("Content-Type", "application/zip")
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", archiveName))

	zipReader, err := os.Open(tempZipPath)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, fmt.Sprintf("Error reading temp zip: %v", err))
		return
	}
	defer zipReader.Close()

	http.ServeContent(c.Writer, c.Request, archiveName, time.Now(), zipReader)
}
