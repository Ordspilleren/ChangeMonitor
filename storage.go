package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type Storage struct {
	Monitor *Monitor
}

func InitStorage(m *Monitor) *Storage {
	return &Storage{Monitor: m}
}

func (s *Storage) GetContent() string {
	filePath := filepath.Join(config.StorageDirectory, s.Monitor.id)

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
	filePath := filepath.Join(dataDir, s.Monitor.id)

	err := ioutil.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		log.Panic(err)
	}
}
