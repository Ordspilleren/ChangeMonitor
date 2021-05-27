package storage

import (
	"io/ioutil"
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

	fileData, err := ioutil.ReadFile(filePath)
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

	err := ioutil.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		log.Panic(err)
	}
}
