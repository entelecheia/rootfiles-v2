package module

import (
	"context"
	"fmt"
)

// MonitoringModule installs Prometheus exporters (opt-in).
type MonitoringModule struct{}

func NewMonitoringModule() *MonitoringModule { return &MonitoringModule{} }
func (m *MonitoringModule) Name() string     { return "monitoring" }

// NodeExporterTextfileDir is where Ubuntu's prometheus-node-exporter reads
// *.prom files; `rootfiles schedule` publishes rootfiles_* metrics there.
const NodeExporterTextfileDir = "/var/lib/prometheus/node-exporter"

const nodeExporterPkg, nodeExporterUnit = "prometheus-node-exporter", "prometheus-node-exporter"

func (m *MonitoringModule) Check(ctx context.Context, rc *RunContext) (*CheckResult, error) {
	var changes []Change
	if rc.Config.Modules.Monitoring.NodeExporter {
		if !rc.APT.IsInstalled(nodeExporterPkg) {
			changes = append(changes, Change{Description: "Install Prometheus node exporter", Command: "apt-get install " + nodeExporterPkg})
		} else if res, err := rc.Runner.Query(ctx, "systemctl", "is-enabled", nodeExporterUnit); err == nil && res.Stdout != "enabled\n" {
			changes = append(changes, Change{Description: "Enable " + nodeExporterUnit, Command: "systemctl enable --now " + nodeExporterUnit})
		}
	}
	return &CheckResult{Satisfied: len(changes) == 0, Changes: changes}, nil
}

func (m *MonitoringModule) Apply(ctx context.Context, rc *RunContext) (*ApplyResult, error) {
	var messages, warnings []string
	changed := false
	if rc.Config.Modules.Monitoring.NodeExporter {
		if !rc.APT.IsInstalled(nodeExporterPkg) {
			if err := rc.APT.Update(ctx); err != nil {
				return nil, fmt.Errorf("apt update: %w", err)
			}
			if err := rc.APT.Install(ctx, []string{nodeExporterPkg}); err != nil {
				return nil, fmt.Errorf("installing %s: %w", nodeExporterPkg, err)
			}
			messages = append(messages, "node exporter installed (:9100)")
			changed = true
		}
		if err := rc.Runner.MkdirAll(NodeExporterTextfileDir, 0755); err != nil {
			return nil, err
		}
		if _, err := rc.Runner.Run(ctx, "systemctl", "enable", "--now", nodeExporterUnit); err != nil {
			warnings = append(warnings, "enabling node exporter: "+firstLine(err.Error()))
		}
	}
	return &ApplyResult{Changed: changed, Messages: messages, Warnings: warnings}, nil
}
