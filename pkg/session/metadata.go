package session

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/jg-phare/goat/pkg/agent"
)

const metadataFile = "metadata.json"

func saveMetadata(dir string, meta agent.SessionMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, metadataFile), data, 0644)
}

func loadMetadata(dir string) (agent.SessionMetadata, error) {
	var meta agent.SessionMetadata
	data, err := os.ReadFile(filepath.Join(dir, metadataFile))
	if err != nil {
		return meta, err
	}
	err = json.Unmarshal(data, &meta)
	return meta, err
}
