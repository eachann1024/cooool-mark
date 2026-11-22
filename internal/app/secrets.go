package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"

	"cooool-mark/internal/store"
)

const (
	// OpenAISecretKey is the credential account key for the OpenAI API key.
	OpenAISecretKey     = "openai-key"
	legacySecretService = "Cooool Mark"
	secretsFileName     = "secrets.json"
)

type localSecrets struct {
	OpenAIKey string `json:"openaiKey,omitempty"`
}

var secretMu sync.Mutex

func secretsFilePath() (string, error) {
	userData, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(userData, secretsFileName), nil
}

func readLocalSecrets(path string) (localSecrets, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return localSecrets{}, false, nil
		}
		return localSecrets{}, false, err
	}
	var s localSecrets
	if err := json.Unmarshal(data, &s); err != nil {
		return localSecrets{}, false, err
	}
	return s, true, nil
}

func writeLocalSecrets(path string, s localSecrets) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return store.WriteAtomic(path, append(data, '\n'))
}

// LoadOpenAIKey reads the OpenAI API key from the app's local secrets file in
// userData so macOS ad-hoc builds never trigger repeated Keychain password prompts.
// If the local file does not exist yet, it migrates any existing key from
// mygo.Secrets (primary or legacy "Cooool Mark" service) once and removes the
// keychain entries.
func LoadOpenAIKey() ([]byte, error) {
	secretMu.Lock()
	defer secretMu.Unlock()

	path, err := secretsFilePath()
	if err == nil {
		if s, ok, readErr := readLocalSecrets(path); readErr == nil && ok {
			if s.OpenAIKey == "" {
				return nil, mygo.ErrSecretNotFound
			}
			return []byte(s.OpenAIKey), nil
		}
	}

	raw, err := mygo.Secrets.Get(OpenAISecretKey)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		legacy := mygo.Secrets.Service(legacySecretService)
		raw, err = legacy.Get(OpenAISecretKey)
	}
	if err != nil {
		if errors.Is(err, mygo.ErrSecretNotFound) && path != "" {
			_ = writeLocalSecrets(path, localSecrets{})
		}
		return nil, err
	}
	if len(raw) == 0 {
		if path != "" {
			_ = writeLocalSecrets(path, localSecrets{})
		}
		deleteKeychainEntries()
		return nil, mygo.ErrSecretNotFound
	}

	if path != "" {
		if writeErr := writeLocalSecrets(path, localSecrets{OpenAIKey: string(raw)}); writeErr == nil {
			deleteKeychainEntries()
		}
	}
	return raw, nil
}

// SaveOpenAIKey writes the OpenAI API key to the local secrets file.
func SaveOpenAIKey(val string) error {
	secretMu.Lock()
	defer secretMu.Unlock()

	path, err := secretsFilePath()
	if err != nil {
		return err
	}
	if err := writeLocalSecrets(path, localSecrets{OpenAIKey: val}); err != nil {
		return err
	}
	if val == "" {
		deleteKeychainEntries()
	}
	return nil
}

// DeleteOpenAIKey clears the OpenAI API key from both the local secrets file
// and any legacy keychain entries so an old key cannot resurface later.
func DeleteOpenAIKey() error {
	secretMu.Lock()
	defer secretMu.Unlock()

	path, err := secretsFilePath()
	if err != nil {
		return err
	}
	if err := writeLocalSecrets(path, localSecrets{}); err != nil {
		return err
	}
	deleteKeychainEntries()
	return nil
}

func deleteKeychainEntries() {
	_ = mygo.Secrets.Delete(OpenAISecretKey)
	_ = mygo.Secrets.Service(legacySecretService).Delete(OpenAISecretKey)
}
