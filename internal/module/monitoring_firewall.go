package module

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

type ufwInboundRule struct {
	Target string
	Source string
}

type ufwVerboseState struct {
	Active         bool
	StatusKnown    bool
	DefaultInbound string
	Rules          []ufwInboundRule
}

func inspectUFWVerbose(ctx context.Context, rc *RunContext) (ufwVerboseState, error) {
	res, err := rc.Runner.Query(ctx, "ufw", "status", "verbose")
	if err != nil {
		return ufwVerboseState{}, err
	}
	state := ufwVerboseState{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		trim := strings.TrimSpace(line)
		lower := strings.ToLower(trim)
		if strings.HasPrefix(lower, "status:") {
			state.StatusKnown = true
			state.Active = strings.TrimSpace(strings.TrimPrefix(lower, "status:")) == "active"
			continue
		}
		if !state.Active {
			continue
		}
		if i := strings.Index(lower, "(incoming)"); i >= 0 {
			before := strings.TrimSpace(strings.TrimSuffix(trim[:i], "Default:"))
			fields := strings.Fields(before)
			if len(fields) > 0 {
				state.DefaultInbound = strings.ToLower(strings.Trim(fields[len(fields)-1], ":"))
			}
			continue
		}
		fields := strings.Fields(trim)
		allowAt := -1
		for i, field := range fields {
			if strings.EqualFold(field, "ALLOW") {
				allowAt = i
				break
			}
		}
		if allowAt >= 0 && allowAt+2 < len(fields) && strings.EqualFold(fields[allowAt+1], "IN") {
			state.Rules = append(state.Rules, ufwInboundRule{Target: fields[0], Source: fields[allowAt+2]})
		}
	}
	return state, nil
}

func preflightExporterFirewall(ctx context.Context, rc *RunContext) error {
	cfg := rc.Config.Modules.Monitoring
	ports := exporterPorts(cfg)
	if len(ports) == 0 || len(cfg.AllowFrom) == 0 || !rc.Runner.CommandExists("ufw") {
		return nil
	}
	state, err := inspectUFWVerbose(ctx, rc)
	if err != nil {
		return fmt.Errorf("checking UFW exporter access rules: %w", err)
	}
	if !state.StatusKnown {
		return fmt.Errorf("cannot determine UFW state before restricting exporter access")
	}
	if !state.Active {
		return nil
	}
	if state.DefaultInbound == "allow" {
		return fmt.Errorf("UFW default incoming policy allows traffic; refusing to claim allow_from restricts exporter access")
	}
	if state.DefaultInbound != "deny" && state.DefaultInbound != "reject" {
		return fmt.Errorf("cannot determine a restrictive UFW incoming policy (got %q)", state.DefaultInbound)
	}
	for _, rule := range state.Rules {
		for _, port := range ports {
			applies, err := ufwRuleAppliesToPort(ctx, rc, rule.Target, port)
			if err != nil {
				return err
			}
			if !applies {
				continue
			}
			if !ufwSourceInAllowFrom(rule.Source, cfg.AllowFrom) {
				return fmt.Errorf("UFW rule %q ALLOW IN %q conflicts with modules.monitoring.allow_from for exporter port %d; adjust it explicitly before applying", rule.Target, rule.Source, port)
			}
		}
	}
	return nil
}

func ufwRuleAppliesToPort(ctx context.Context, rc *RunContext, target string, port int) (bool, error) {
	value := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(target, "/tcp"), "/udp"))
	if value == "anywhere" || value == "any" {
		return true, nil
	}
	matched, known := numericUFWPortTarget(value, port)
	if known {
		return matched, nil
	}
	// UFW application profiles can map a name to a port or range. Resolve them
	// read-only so a named broad rule cannot bypass the port preflight.
	res, err := rc.Runner.Query(ctx, "ufw", "app", "info", target)
	if err != nil {
		return false, fmt.Errorf("cannot resolve UFW application rule %q while checking exporter port %d: %w", target, port, err)
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		trim := strings.TrimSpace(line)
		if i := strings.Index(trim, "/"); i > 0 {
			if applies, known := numericUFWPortTarget(strings.ToLower(trim[:i]), port); known && applies {
				return true, nil
			}
		}
	}
	return false, nil
}

func numericUFWPortTarget(target string, port int) (applies, known bool) {
	parts := strings.Split(target, ",")
	for _, part := range parts {
		bounds := strings.SplitN(part, ":", 2)
		start, err := strconv.Atoi(bounds[0])
		if err != nil {
			return false, false
		}
		end := start
		if len(bounds) == 2 {
			end, err = strconv.Atoi(bounds[1])
			if err != nil {
				return false, false
			}
		}
		if port >= start && port <= end {
			applies = true
		}
	}
	return applies, true
}

func ufwSourceInAllowFrom(source string, allowFrom []string) bool {
	source = strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(source), "(v6)"), "/32")
	source = strings.TrimSuffix(source, "/128")
	if addr, err := netip.ParseAddr(source); err == nil {
		source = addr.String()
	}
	for _, allowed := range allowFrom {
		prefix, err := netip.ParsePrefix(allowed)
		if err != nil {
			continue
		}
		if prefix.Bits() == prefix.Addr().BitLen() {
			candidate := prefix.Addr().String()
			if source == candidate || source == candidate+"/"+strconv.Itoa(prefix.Bits()) {
				return true
			}
			continue
		}
		if source == prefix.String() {
			return true
		}
	}
	return false
}

func ufwRestrictedPortRuleExists(ctx context.Context, rc *RunContext, port int) bool {
	cfg := rc.Config.Modules.Monitoring
	if len(cfg.AllowFrom) == 0 {
		return false
	}
	state, err := inspectUFWVerbose(ctx, rc)
	if err != nil || !state.StatusKnown || !state.Active || (state.DefaultInbound != "deny" && state.DefaultInbound != "reject") {
		return false
	}
	found := false
	for _, rule := range state.Rules {
		applies, err := ufwRuleAppliesToPort(ctx, rc, rule.Target, port)
		if err != nil {
			return false
		}
		if !applies {
			continue
		}
		if !ufwSourceInAllowFrom(rule.Source, cfg.AllowFrom) {
			return false
		}
		found = true
	}
	return found
}
