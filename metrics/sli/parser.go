package sli

import (
	"bufio"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// ParseSLIStream parses Prometheus text exposition from /metrics/slis, extracting
// kubernetes_healthcheck and kubernetes_healthchecks_total series.
func ParseSLIStream(r io.Reader, component, target string) (ComponentSLI, bool, error) {
	checks := make(map[string]*HealthCheck)
	getCheck := func(name, typ string) *HealthCheck {
		key := typ + "\x00" + name
		c, ok := checks[key]
		if !ok {
			c = &HealthCheck{
				Name:    name,
				Type:    typ,
				Healthy: true,
			}
			checks[key] = c
		}
		return c
	}

	parsedAny := false
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		metricName, rawLabels, val, ok := parsePromLine(line)
		if !ok {
			continue
		}
		switch metricName {
		case "kubernetes_healthcheck":
			lbls := parsePromLabels(rawLabels)
			name := lbls["name"]
			if name == "" {
				continue
			}
			chk := getCheck(name, lbls["type"])
			chk.Healthy = val >= 1
			parsedAny = true

		case "kubernetes_healthchecks_total":
			lbls := parsePromLabels(rawLabels)
			name := lbls["name"]
			if name == "" {
				continue
			}
			chk := getCheck(name, lbls["type"])
			switch strings.ToLower(lbls["status"]) {
			case "success":
				chk.SuccessCount += val
			case "error", "failure":
				chk.ErrorCount += val
			}
			parsedAny = true
		}
	}

	if err := scanner.Err(); err != nil {
		return ComponentSLI{}, false, err
	}

	res := ComponentSLI{
		Component:   component,
		Target:      target,
		Healthy:     true,
		TotalChecks: len(checks),
		Checks:      make([]HealthCheck, 0, len(checks)),
	}
	for _, c := range checks {
		res.Checks = append(res.Checks, *c)
		if !c.Healthy {
			res.Healthy = false
			res.FailingChecks = append(res.FailingChecks, *c)
		}
	}
	sortChecks := func(list []HealthCheck) {
		sort.Slice(list, func(i, j int) bool {
			if list[i].Type != list[j].Type {
				return list[i].Type < list[j].Type
			}
			return list[i].Name < list[j].Name
		})
	}
	sortChecks(res.Checks)
	sortChecks(res.FailingChecks)

	return res, parsedAny, nil
}

// ParseProbesStream parses Prometheus text exposition from kubelet /metrics/probes,
// extracting prober_probe_total series grouped by namespace/pod.
func ParseProbesStream(r io.Reader) (map[string][]ProbeStats, bool, error) {
	byKey := make(map[string]*ProbeStats)
	parsedAny := false

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		metricName, rawLabels, val, ok := parsePromLine(line)
		if !ok || metricName != "prober_probe_total" {
			continue
		}
		lbls := parsePromLabels(rawLabels)
		ns := lbls["namespace"]
		pod := lbls["pod"]
		container := lbls["container"]
		probeType := lbls["probe_type"]
		if pod == "" || probeType == "" {
			continue
		}
		parsedAny = true

		key := ns + "\x00" + pod + "\x00" + container + "\x00" + probeType
		st, exists := byKey[key]
		if !exists {
			st = &ProbeStats{
				Namespace: ns,
				Pod:       pod,
				Container: container,
				ProbeType: probeType,
			}
			byKey[key] = st
		}
		switch strings.ToLower(lbls["result"]) {
		case "successful", "success":
			st.SuccessCount += val
		case "failed", "failure":
			st.FailureCount += val
		default:
			st.UnknownCount += val
		}
		st.Failing = st.FailureCount > 0 || st.UnknownCount > 0
	}

	if err := scanner.Err(); err != nil {
		return nil, false, err
	}

	out := make(map[string][]ProbeStats)
	for _, st := range byKey {
		podKey := st.Namespace + "/" + st.Pod
		out[podKey] = append(out[podKey], *st)
	}
	for podKey := range out {
		list := out[podKey]
		sort.Slice(list, func(i, j int) bool {
			if list[i].Container != list[j].Container {
				return list[i].Container < list[j].Container
			}
			return list[i].ProbeType < list[j].ProbeType
		})
		out[podKey] = list
	}
	return out, parsedAny, nil
}

func parsePromLine(line string) (metricName, rawLabels string, val float64, ok bool) {
	braceStart := strings.IndexByte(line, '{')
	var rest string
	if braceStart >= 0 {
		metricName = strings.TrimSpace(line[:braceStart])
		braceEnd := findClosingBrace(line, braceStart+1)
		if braceEnd < 0 {
			return "", "", 0, false
		}
		rawLabels = line[braceStart+1 : braceEnd]
		rest = strings.TrimSpace(line[braceEnd+1:])
	} else {
		sp := strings.IndexAny(line, " \t")
		if sp <= 0 {
			return "", "", 0, false
		}
		metricName = line[:sp]
		rest = strings.TrimSpace(line[sp+1:])
	}
	if metricName == "" || rest == "" {
		return "", "", 0, false
	}
	valField := rest
	if sp := strings.IndexAny(rest, " \t"); sp > 0 {
		valField = rest[:sp]
	}
	parsed, err := strconv.ParseFloat(valField, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return "", "", 0, false
	}
	return metricName, rawLabels, parsed, true
}

func findClosingBrace(s string, start int) int {
	inQuotes := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inQuotes {
			escaped = true
			continue
		}
		if c == '"' {
			inQuotes = !inQuotes
			continue
		}
		if c == '}' && !inQuotes {
			return i
		}
	}
	return -1
}

func parsePromLabels(raw string) map[string]string {
	out := make(map[string]string, 5)
	i := 0
	n := len(raw)
	for i < n {
		for i < n && (raw[i] == ' ' || raw[i] == '\t' || raw[i] == ',') {
			i++
		}
		if i >= n {
			break
		}
		eq := strings.IndexByte(raw[i:], '=')
		if eq <= 0 {
			break
		}
		k := strings.TrimSpace(raw[i : i+eq])
		i += eq + 1
		for i < n && (raw[i] == ' ' || raw[i] == '\t') {
			i++
		}
		if i >= n || raw[i] != '"' {
			break
		}
		i++
		var vb strings.Builder
		for i < n {
			c := raw[i]
			if c == '\\' && i+1 < n {
				i++
				switch raw[i] {
				case 'n':
					vb.WriteByte('\n')
				case '"':
					vb.WriteByte('"')
				case '\\':
					vb.WriteByte('\\')
				default:
					vb.WriteByte(raw[i])
				}
				i++
				continue
			}
			if c == '"' {
				i++
				break
			}
			vb.WriteByte(c)
			i++
		}
		if k != "" {
			out[k] = vb.String()
		}
	}
	return out
}
