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
	data, err := c.Effective()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Effective encodes the resolved config without extends or the inline
// tunnel token. Fingerprint hashes these bytes, and apply keeps them as the
// copy subcommands reuse, so the two always match.
func (c *Config) Effective() ([]byte, error) {
	copy := *c
	copy.Extends = ""
	copy.Modules.Cloudflared.TunnelToken = ""
	return yaml.Marshal(&copy)
}
