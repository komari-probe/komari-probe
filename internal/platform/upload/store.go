// Package upload provides the shared chunked archive upload flow.
package upload

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// DefaultMaxSize bounds an upload session created against DefaultStore.
// It matches the backup feature's own archive size limit, but the two are
// independent constants: this package has no business awareness of backup.
const DefaultMaxSize int64 = 4 << 30 // 4 GiB

type Purpose string

const (
	PurposeBackup Purpose = "backup"
	PurposePlugin Purpose = "plugin"
	PurposeTheme  Purpose = "theme"
)

var ErrNotFound = errors.New("upload not found")

type Metadata struct {
	Purpose  Purpose `json:"purpose"`
	Size     int64   `json:"size"`
	Filename string  `json:"filename"`
}

type Session struct {
	ID          string
	Metadata    Metadata
	Directory   string
	ArchivePath string
}

type Store struct {
	Root    string
	MaxSize int64
}

var DefaultStore = &Store{
	Root:    filepath.Join(".", "data", ".uploading"),
	MaxSize: DefaultMaxSize,
}

func (s *Store) Init(purpose Purpose, filename string, size int64) (Session, error) {
	if !isKnownPurpose(purpose) {
		return Session{}, fmt.Errorf("invalid upload purpose")
	}
	if size <= 0 || size > s.MaxSize {
		return Session{}, fmt.Errorf("size must be between 1 and %d bytes", s.MaxSize)
	}

	id := uuid.NewString()
	directory := filepath.Join(s.Root, id)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return Session{}, fmt.Errorf("create upload directory: %w", err)
	}
	metadata := Metadata{Purpose: purpose, Size: size, Filename: filename}
	data, err := json.Marshal(metadata)
	if err != nil {
		_ = os.RemoveAll(directory)
		return Session{}, fmt.Errorf("encode upload metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "upload.json"), data, 0600); err != nil {
		_ = os.RemoveAll(directory)
		return Session{}, fmt.Errorf("write upload metadata: %w", err)
	}
	return Session{ID: id, Metadata: metadata, Directory: directory}, nil
}

func (s *Store) Merge(uploadID string) (Session, error) {
	session, err := s.load(uploadID)
	if err != nil {
		return Session{}, err
	}

	temporary, err := os.CreateTemp(session.Directory, ".merged-*.zip")
	if err != nil {
		return Session{}, fmt.Errorf("create merged archive: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	for index := int64(0); index < chunkCount(session.Metadata.Size); index++ {
		chunkPath := filepath.Join(session.Directory, chunkFilename(index))
		info, err := os.Stat(chunkPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return Session{}, fmt.Errorf("chunk %d is missing", index)
			}
			return Session{}, fmt.Errorf("read chunk %d: %w", index, err)
		}
		expectedSize, _ := expectedChunkSize(session.Metadata.Size, index)
		if info.Size() != expectedSize {
			return Session{}, fmt.Errorf("chunk %d has size %d, want %d", index, info.Size(), expectedSize)
		}
		chunk, err := os.Open(chunkPath)
		if err != nil {
			return Session{}, fmt.Errorf("open chunk %d: %w", index, err)
		}
		_, copyErr := io.Copy(temporary, chunk)
		closeErr := chunk.Close()
		if copyErr != nil {
			return Session{}, fmt.Errorf("merge chunk %d: %w", index, copyErr)
		}
		if closeErr != nil {
			return Session{}, fmt.Errorf("close chunk %d: %w", index, closeErr)
		}
	}
	if err := temporary.Close(); err != nil {
		return Session{}, fmt.Errorf("close merged archive: %w", err)
	}

	archivePath := filepath.Join(session.Directory, "archive.zip")
	if err := os.Rename(temporaryPath, archivePath); err != nil {
		return Session{}, fmt.Errorf("publish merged archive: %w", err)
	}
	session.ArchivePath = archivePath
	return session, nil
}

func (s *Store) Cancel(uploadID string) error {
	if !validUploadID(uploadID) {
		return fmt.Errorf("invalid upload id")
	}
	if err := os.RemoveAll(filepath.Join(s.Root, uploadID)); err != nil {
		return fmt.Errorf("remove upload: %w", err)
	}
	return nil
}

func (s *Store) load(uploadID string) (Session, error) {
	if !validUploadID(uploadID) {
		return Session{}, fmt.Errorf("invalid upload id")
	}
	directory := filepath.Join(s.Root, uploadID)
	data, err := os.ReadFile(filepath.Join(directory, "upload.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Session{}, ErrNotFound
		}
		return Session{}, fmt.Errorf("read upload metadata: %w", err)
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Session{}, fmt.Errorf("read upload metadata: %w", err)
	}
	if !isKnownPurpose(metadata.Purpose) || metadata.Size <= 0 || metadata.Size > s.MaxSize {
		return Session{}, fmt.Errorf("invalid upload metadata")
	}
	return Session{ID: uploadID, Metadata: metadata, Directory: directory}, nil
}

func isKnownPurpose(purpose Purpose) bool {
	return purpose == PurposeBackup || purpose == PurposePlugin || purpose == PurposeTheme
}

func validUploadID(uploadID string) bool {
	_, err := uuid.Parse(uploadID)
	return err == nil
}
