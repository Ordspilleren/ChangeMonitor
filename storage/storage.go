package storage

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type Storage struct {
	ID        string
	Directory string
}

func InitStorage(id string, directory string) *Storage {
	return &Storage{ID: id, Directory: directory}
}

func (s *Storage) GetContent() string {
	filePath := filepath.Join(s.Directory, s.ID)

	os.Mkdir(s.Directory, os.ModePerm)

	fileData, err := ioutil.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Print("File did not exist, returning empty string.")
			return ""
		}
	}

	return string(fileData)
}

func (s *Storage) WriteContent(content string) {
	filePath := filepath.Join(s.Directory, s.ID)

	err := ioutil.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		log.Panic(err)
	}
}
