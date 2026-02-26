package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/georgele/devctl/internal/vault/crypto"
)

// atomicWriteFile writes data to a file atomically by writing to a temp file
// and renaming. This prevents partial writes from corrupting the file.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

const (
	VaultDir    = ".envsafe"
	VaultFile   = "vault.enc"
	ConfigFile  = "config.json"
	AuditFile   = "audit.log"
	DefaultEnv  = "development"
)

// RotationEntry records a previous value for a rotated secret.
type RotationEntry struct {
	Value     string    `json:"value"`
	RotatedAt time.Time `json:"rotated_at"`
}

// SecretEntry holds a secret value and its rotation history.
type SecretEntry struct {
	Value    string          `json:"value"`
	History  []RotationEntry `json:"history,omitempty"`
	SetAt    time.Time       `json:"set_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// VaultData is the decrypted structure stored in vault.enc.
// Map of environment name → key → secret entry.
type VaultData struct {
	Environments map[string]map[string]*SecretEntry `json:"environments"`
}

// VaultConfig is the unencrypted metadata in config.json.
type VaultConfig struct {
	Project      string            `json:"project"`
	Environments []string          `json:"environments"`
	CreatedAt    time.Time         `json:"created_at"`
	TeamKeys     map[string]string `json:"team_keys,omitempty"` // email → public key (base64)
}

// Vault manages encrypted secrets for a project.
type Vault struct {
	Root     string      // Project root directory
	Config   VaultConfig
	data     *VaultData
	key      []byte // Derived encryption key (nil when locked)
	salt     []byte // Argon2id salt
	mu       sync.RWMutex
}

// VaultPath returns the path to the .envsafe directory.
func VaultPath(projectRoot string) string {
	return filepath.Join(projectRoot, VaultDir)
}

// Exists checks if a vault exists in the project root.
func Exists(projectRoot string) bool {
	_, err := os.Stat(filepath.Join(VaultPath(projectRoot), ConfigFile))
	return err == nil
}

// Init creates a new vault in the project root.
func Init(projectRoot, projectName, password string) (*Vault, error) {
	vaultDir := VaultPath(projectRoot)

	if _, err := os.Stat(filepath.Join(vaultDir, ConfigFile)); err == nil {
		return nil, fmt.Errorf("vault already exists in %s", vaultDir)
	}

	if err := os.MkdirAll(vaultDir, 0700); err != nil {
		return nil, fmt.Errorf("creating vault directory: %w", err)
	}

	salt, err := crypto.GenerateSalt()
	if err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}

	key, err := crypto.DeriveKey(password, salt)
	if err != nil {
		return nil, fmt.Errorf("deriving key: %w", err)
	}

	cfg := VaultConfig{
		Project:      projectName,
		Environments: []string{DefaultEnv},
		CreatedAt:    time.Now().UTC(),
	}

	data := &VaultData{
		Environments: map[string]map[string]*SecretEntry{
			DefaultEnv: {},
		},
	}

	v := &Vault{
		Root:   projectRoot,
		Config: cfg,
		data:   data,
		key:    key,
		salt:   salt,
	}

	if err := v.saveConfig(); err != nil {
		return nil, err
	}

	if err := v.saveVault(); err != nil {
		return nil, err
	}

	return v, nil
}

// Open loads an existing vault (locked — call Unlock to decrypt).
func Open(projectRoot string) (*Vault, error) {
	vaultDir := VaultPath(projectRoot)

	cfgData, err := os.ReadFile(filepath.Join(vaultDir, ConfigFile))
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg VaultConfig
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &Vault{
		Root:   projectRoot,
		Config: cfg,
	}, nil
}

// Unlock decrypts the vault with the given password.
func (v *Vault) Unlock(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	vaultDir := VaultPath(v.Root)

	raw, err := os.ReadFile(filepath.Join(vaultDir, VaultFile))
	if err != nil {
		return fmt.Errorf("reading vault: %w", err)
	}

	// First 16 bytes are the salt
	if len(raw) < crypto.SaltLen {
		return fmt.Errorf("vault file too short")
	}

	v.salt = raw[:crypto.SaltLen]
	encrypted := raw[crypto.SaltLen:]

	key, err := crypto.DeriveKey(password, v.salt)
	if err != nil {
		return fmt.Errorf("deriving key: %w", err)
	}

	plaintext, err := crypto.Decrypt(key, encrypted)
	if err != nil {
		return fmt.Errorf("wrong password or corrupted vault")
	}
	defer func() { for i := range plaintext { plaintext[i] = 0 } }()

	var data VaultData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return fmt.Errorf("parsing vault data: %w", err)
	}

	v.key = key
	v.data = &data
	return nil
}

// Lock clears the decryption key and data from memory.
func (v *Vault) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Zero out the key
	for i := range v.key {
		v.key[i] = 0
	}
	v.key = nil
	// Zero out the salt
	for i := range v.salt {
		v.salt[i] = 0
	}
	v.salt = nil
	v.data = nil
}

// IsUnlocked returns whether the vault is currently unlocked.
func (v *Vault) IsUnlocked() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return v.key != nil && v.data != nil
}

// Set stores a secret in the given environment.
func (v *Vault) Set(env, key, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.key == nil || v.data == nil {
		return fmt.Errorf("vault is locked")
	}

	if _, ok := v.data.Environments[env]; !ok {
		v.data.Environments[env] = make(map[string]*SecretEntry)
		if err := v.addEnvironment(env); err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	if existing, ok := v.data.Environments[env][key]; ok {
		existing.Value = value
		existing.UpdatedAt = now
	} else {
		v.data.Environments[env][key] = &SecretEntry{
			Value:     value,
			SetAt:     now,
			UpdatedAt: now,
		}
	}

	return v.saveVault()
}

// Get retrieves a secret from the given environment.
func (v *Vault) Get(env, key string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.key == nil || v.data == nil {
		return "", fmt.Errorf("vault is locked")
	}

	envSecrets, ok := v.data.Environments[env]
	if !ok {
		return "", fmt.Errorf("environment %q not found", env)
	}

	entry, ok := envSecrets[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in environment %q", key, env)
	}

	return entry.Value, nil
}

// List returns all keys in the given environment (sorted).
func (v *Vault) List(env string) ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.key == nil || v.data == nil {
		return nil, fmt.Errorf("vault is locked")
	}

	envSecrets, ok := v.data.Environments[env]
	if !ok {
		return nil, fmt.Errorf("environment %q not found", env)
	}

	keys := make([]string, 0, len(envSecrets))
	for k := range envSecrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

// Remove deletes a secret from the given environment.
func (v *Vault) Remove(env, key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.key == nil || v.data == nil {
		return fmt.Errorf("vault is locked")
	}

	envSecrets, ok := v.data.Environments[env]
	if !ok {
		return fmt.Errorf("environment %q not found", env)
	}

	if _, ok := envSecrets[key]; !ok {
		return fmt.Errorf("key %q not found in environment %q", key, env)
	}

	delete(envSecrets, key)
	return v.saveVault()
}

// Env returns all key-value pairs for the given environment.
func (v *Vault) Env(env string) (map[string]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.key == nil || v.data == nil {
		return nil, fmt.Errorf("vault is locked")
	}

	envSecrets, ok := v.data.Environments[env]
	if !ok {
		return nil, fmt.Errorf("environment %q not found", env)
	}

	result := make(map[string]string, len(envSecrets))
	for k, entry := range envSecrets {
		result[k] = entry.Value
	}
	return result, nil
}

// Rotate updates a secret's value and stores the previous value in history.
func (v *Vault) Rotate(env, key, newValue string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.key == nil || v.data == nil {
		return fmt.Errorf("vault is locked")
	}

	envSecrets, ok := v.data.Environments[env]
	if !ok {
		return fmt.Errorf("environment %q not found", env)
	}

	entry, ok := envSecrets[key]
	if !ok {
		return fmt.Errorf("key %q not found in environment %q", key, env)
	}

	now := time.Now().UTC()
	entry.History = append(entry.History, RotationEntry{
		Value:     entry.Value,
		RotatedAt: now,
	})
	entry.Value = newValue
	entry.UpdatedAt = now

	return v.saveVault()
}

// ChangePassword re-encrypts the vault with a new password.
// The vault must be unlocked first.
func (v *Vault) ChangePassword(newPassword string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.key == nil || v.data == nil {
		return fmt.Errorf("vault is locked")
	}

	newSalt, err := crypto.GenerateSalt()
	if err != nil {
		return fmt.Errorf("generating salt: %w", err)
	}

	newKey, err := crypto.DeriveKey(newPassword, newSalt)
	if err != nil {
		return fmt.Errorf("deriving key: %w", err)
	}

	// Zero old key
	for i := range v.key {
		v.key[i] = 0
	}

	v.key = newKey
	v.salt = newSalt

	return v.saveVault()
}

// ListEnvironments returns all environment names (sorted).
func (v *Vault) ListEnvironments() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	envs := make([]string, len(v.Config.Environments))
	copy(envs, v.Config.Environments)
	sort.Strings(envs)
	return envs
}

// ImportSecrets imports a map of key-value pairs into the given environment.
func (v *Vault) ImportSecrets(env string, secrets map[string]string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.key == nil || v.data == nil {
		return fmt.Errorf("vault is locked")
	}

	if _, ok := v.data.Environments[env]; !ok {
		v.data.Environments[env] = make(map[string]*SecretEntry)
		if err := v.addEnvironment(env); err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	for k, val := range secrets {
		v.data.Environments[env][k] = &SecretEntry{
			Value:     val,
			SetAt:     now,
			UpdatedAt: now,
		}
	}

	return v.saveVault()
}

func (v *Vault) addEnvironment(env string) error {
	for _, e := range v.Config.Environments {
		if e == env {
			return nil
		}
	}
	v.Config.Environments = append(v.Config.Environments, env)
	return v.saveConfig()
}

func (v *Vault) saveConfig() error {
	vaultDir := VaultPath(v.Root)

	data, err := json.MarshalIndent(v.Config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')
	return atomicWriteFile(filepath.Join(vaultDir, ConfigFile), data, 0600)
}

func (v *Vault) saveVault() error {
	// Note: callers must hold v.mu; use direct field check to avoid deadlock
	if v.key == nil || v.data == nil {
		return fmt.Errorf("vault is locked")
	}

	vaultDir := VaultPath(v.Root)

	plaintext, err := json.Marshal(v.data)
	if err != nil {
		return fmt.Errorf("marshaling vault data: %w", err)
	}
	defer func() { for i := range plaintext { plaintext[i] = 0 } }()

	encrypted, err := crypto.Encrypt(v.key, plaintext)
	if err != nil {
		return fmt.Errorf("encrypting vault: %w", err)
	}

	// Prepend salt to encrypted data
	out := make([]byte, 0, len(v.salt)+len(encrypted))
	out = append(out, v.salt...)
	out = append(out, encrypted...)

	return atomicWriteFile(filepath.Join(vaultDir, VaultFile), out, 0600)
}
