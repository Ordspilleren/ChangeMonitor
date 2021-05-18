package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type Storage struct {
	ID string
}

func InitStorage(id string) *Storage {
	return &Storage{ID: id}
}

func (s *Storage) GetContent() string {
	filePath := filepath.Join(config.StorageDirectory, s.ID)

	os.Mkdir(config.StorageDirectory, os.ModePerm)

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
	dataDir := "data"
	filePath := filepath.Join(dataDir, s.ID)

	err := ioutil.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		log.Panic(err)
	}
}
