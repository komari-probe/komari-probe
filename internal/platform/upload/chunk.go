package upload

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

const ChunkSize int64 = 5 * 1024 * 1024

func (s *Store) SaveChunk(uploadID string, index int64, source io.Reader) error {
	session, err := s.load(uploadID)
	if err != nil {
		return err
	}
	expectedSize, err := expectedChunkSize(session.Metadata.Size, index)
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp(session.Directory, fmt.Sprintf(".%d-*.part", index))
	if err != nil {
		return fmt.Errorf("create chunk: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	written, copyErr := io.Copy(temporary, io.LimitReader(source, ChunkSize+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return fmt.Errorf("write chunk: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close chunk: %w", closeErr)
	}
	if written != expectedSize {
		return fmt.Errorf("chunk %d has size %d, want %d", index, written, expectedSize)
	}

	chunkPath := filepath.Join(session.Directory, chunkFilename(index))
	if err := os.Remove(chunkPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("replace chunk: %w", err)
	}
	if err := os.Rename(temporaryPath, chunkPath); err != nil {
		return fmt.Errorf("publish chunk: %w", err)
	}
	return nil
}

func chunkCount(size int64) int64 {
	return (size + ChunkSize - 1) / ChunkSize
}

func expectedChunkSize(size, index int64) (int64, error) {
	if index < 0 || index >= chunkCount(size) {
		return 0, fmt.Errorf("invalid chunk index")
	}
	if index == chunkCount(size)-1 {
		return size - index*ChunkSize, nil
	}
	return ChunkSize, nil
}

func chunkFilename(index int64) string {
	return strconv.FormatInt(index, 10) + ".part"
}
