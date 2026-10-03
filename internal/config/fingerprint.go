package config

import (
	"crypto/sha256"
	"encoding/hex"

	"gopkg.in/yaml.v3"
)

// Fingerprint identifies effective configuration without exposing inline
// secret material. Runtime hardware detection is omitted by YAML tags.
func (c *Config) Fingerprint() (string, error) {
	if c == nil {
		return "", nil
	}
	copy := *c
	copy.Extends = ""
	copy.Modules.Cloudflared.TunnelToken = ""
	data, err := yaml.Marshal(&copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
