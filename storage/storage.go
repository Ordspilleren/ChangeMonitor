package storage

import (
	"log"
	"os"
	"path/filepath"
)

type Storage struct {
	Directory string
}

func InitStorage(directory string) *Storage {
	return &Storage{Directory: directory}
}

func (s *Storage) GetContent(id string) string {
	filePath := filepath.Join(s.Directory, id)

	os.Mkdir(s.Directory, os.ModePerm)

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Print("File did not exist, returning empty string.")
			return ""
		}
	}

	return string(fileData)
}

func (s *Storage) WriteContent(id string, content string) {
	filePath := filepath.Join(s.Directory, id)

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		log.Panic(err)
	}
}

// Cleanup removes state files for any monitor ID not present in activeIDs.
// Files in the storage directory that do not match an active ID are deleted.
func (s *Storage) Cleanup(activeIDs []string) error {
	entries, err := os.ReadDir(s.Directory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	active := make(map[string]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		active[id] = struct{}{}
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, ok := active[e.Name()]; !ok {
			path := filepath.Join(s.Directory, e.Name())
			if err := os.Remove(path); err != nil {
				log.Printf("storage: cleanup: %v", err)
			} else {
				log.Printf("storage: removed orphan state file %q", e.Name())
			}
		}
	}
	return nil
}
