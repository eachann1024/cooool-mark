package app

import (
	"errors"
	"sync"

	"github.com/egoist/mygo"
)

const (
	// OpenAISecretKey is the credential account key for the OpenAI API key.
	OpenAISecretKey     = "openai-key"
	legacySecretService = "Cooool Mark"
)

var secretMu sync.Mutex

// LoadOpenAIKey reads the OpenAI API key from the app's primary credential
// store. Only when the primary store reports ErrSecretNotFound does it check
// the legacy "Cooool Mark" service name (used before packageIdentifier was
// baked into Windows/Linux release builds), migrate it to the primary store,
// verify the migrated write, and delete the legacy entry.
//
// All Load/Save/Delete calls are serialized under secretMu so a concurrent
// migration can never overwrite a newly saved or cleared API key.
func LoadOpenAIKey() ([]byte, error) {
	secretMu.Lock()
	defer secretMu.Unlock()

	raw, err := mygo.Secrets.Get(OpenAISecretKey)
	if err == nil || !errors.Is(err, mygo.ErrSecretNotFound) {
		return raw, err
	}

	legacy := mygo.Secrets.Service(legacySecretService)
	oldVal, oldErr := legacy.Get(OpenAISecretKey)
	if oldErr != nil {
		return nil, err
	}
	if len(oldVal) == 0 {
		_ = legacy.Delete(OpenAISecretKey)
		return nil, mygo.ErrSecretNotFound
	}

	// Because primary returned ErrSecretNotFound and legacy succeeded, primary
	// and legacy are distinct stores. Delete legacy only after verifying readback.
	if setErr := mygo.Secrets.Set(OpenAISecretKey, oldVal); setErr == nil {
		if check, getErr := mygo.Secrets.Get(OpenAISecretKey); getErr == nil && string(check) == string(oldVal) {
			_ = legacy.Delete(OpenAISecretKey)
		}
	}
	return oldVal, nil
}

// SaveOpenAIKey writes the OpenAI API key to the primary credential store.
func SaveOpenAIKey(val string) error {
	secretMu.Lock()
	defer secretMu.Unlock()

	if val == "" {
		return deleteOpenAIKeyLocked()
	}
	return mygo.Secrets.Set(OpenAISecretKey, []byte(val))
}

// DeleteOpenAIKey removes the OpenAI API key from both the primary and legacy
// credential stores so an old key cannot resurface later.
func DeleteOpenAIKey() error {
	secretMu.Lock()
	defer secretMu.Unlock()

	return deleteOpenAIKeyLocked()
}

func deleteOpenAIKeyLocked() error {
	err := mygo.Secrets.Delete(OpenAISecretKey)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		err = nil
	}
	legacyErr := mygo.Secrets.Service(legacySecretService).Delete(OpenAISecretKey)
	if errors.Is(legacyErr, mygo.ErrSecretNotFound) || errors.Is(legacyErr, errors.ErrUnsupported) {
		legacyErr = nil
	}
	if err != nil {
		return err
	}
	return legacyErr
}
