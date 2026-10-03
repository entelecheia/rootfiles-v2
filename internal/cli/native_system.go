package cli

import "github.com/entelecheia/rootfiles-v2/internal/config"

// Overridable in unit tests so native command tests never depend on the host distro.
var detectSystem = config.DetectSystem
