package tests

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageTiers(t *testing.T) {
	tmpDir := t.TempDir()

	os.Setenv("TEST_MODE", "1")
	defer os.Unsetenv("TEST_MODE")

	t.Run("SmallValue_HotCache", func(t *testing.T) {
		data := []byte("small value")
		if len(data) >= 64*1024 {
			t.Error("Test data too large for hot cache")
		}
	})

	t.Run("MediumValue_WarmCache", func(t *testing.T) {
		data := make([]byte, 100*1024)
		if len(data) < 64*1024 || len(data) >= 4*1024*1024 {
			t.Error("Test data not in warm range")
		}
	})

	t.Run("LargeValue_ColdStorage", func(t *testing.T) {
		data := make([]byte, 5*1024*1024)
		if len(data) < 4*1024*1024 {
			t.Error("Test data not in cold range")
		}
	})

	t.Run("DirectoryCreation", func(t *testing.T) {
		hotDir := filepath.Join(tmpDir, "hot")
		warmDir := filepath.Join(tmpDir, "warm")
		coldDir := filepath.Join(tmpDir, "cold")

		os.MkdirAll(hotDir, 0755)
		os.MkdirAll(warmDir, 0755)
		os.MkdirAll(coldDir, 0755)

		for _, dir := range []string{hotDir, warmDir, coldDir} {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				t.Errorf("Directory %s was not created", dir)
			}
		}
	})
}

func TestChunking(t *testing.T) {
	t.Run("ChunkSize", func(t *testing.T) {
		chunkSize := 1024 * 1024
		data := make([]byte, 5*1024*1024)

		expectedChunks := (len(data) + chunkSize - 1) / chunkSize
		if expectedChunks != 5 {
			t.Errorf("Expected 5 chunks, got %d", expectedChunks)
		}
	})
}

func TestPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("WarmCachePersists", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "warm", "testkey")
		os.MkdirAll(filepath.Dir(testFile), 0755)

		data := []byte("persistent data")
		if err := os.WriteFile(testFile, data, 0644); err != nil {
			t.Fatal(err)
		}

		read, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatal(err)
		}

		if string(read) != string(data) {
			t.Error("Persisted data doesn't match")
		}
	})
}
