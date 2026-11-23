package ksm

import (
	"bufio"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// allowlistedMetrics limits parsing strictly to high-value governance and DRA series
// so large KSM payloads are scanned with minimal allocations.
var allowlistedMetrics = map[string]struct{}{
	"kube_horizontalpodautoscaler_info":                       {},
	"kube_horizontalpodautoscaler_spec_min_replicas":          {},
	"kube_horizontalpodautoscaler_spec_max_replicas":          {},
	"kube_horizontalpodautoscaler_status_current_replicas":    {},
	"kube_horizontalpodautoscaler_status_desired_replicas":    {},
	"kube_horizontalpodautoscaler_status_condition":           {},
	"kube_poddisruptionbudget_status_current_healthy":         {},
	"kube_poddisruptionbudget_status_desired_healthy":         {},
	"kube_poddisruptionbudget_status_expected_pods":           {},
	"kube_poddisruptionbudget_status_pod_disruptions_allowed": {},
	"kube_resourcequota":                                      {},
	"kube_cronjob_spec_suspend":                               {},
	"kube_cronjob_status_active":                              {},
	"kube_cronjob_status_last_schedule_time":                  {},
	"kube_cronjob_next_schedule_time":                         {},
	"kube_pod_status_disruption_reason":                       {},
	"kube_pod_resourceclaim_info":                             {},
}

// ParseStream parses a Prometheus text-exposition stream from kube-state-metrics,
// discarding non-allowlisted metrics before label parsing.
func ParseStream(r io.Reader, endpoint string, now time.Time) (*Snapshot, error) {
	if now.IsZero() {
		now = time.Now()
	}

	hpas := make(map[string]*HPAMetrics)
	pdbs := make(map[string]*PDBMetrics)
	quotas := make(map[string]*ResourceQuotaMetrics)
	cronJobs := make(map[string]*CronJobMetrics)
	disruptions := make(map[string]string)
	claims := make(map[string][]ResourceClaimRef)
	claimSeen := make(map[string]struct{})

	getHPA := func(ns, name string) *HPAMetrics {
		key := ns + "/" + name
		h, ok := hpas[key]
		if !ok {
			h = &HPAMetrics{Namespace: ns, Name: name}
			hpas[key] = h
		}
		return h
	}
	getPDB := func(ns, name string) *PDBMetrics {
		key := ns + "/" + name
		p, ok := pdbs[key]
		if !ok {
			p = &PDBMetrics{Namespace: ns, Name: name}
			pdbs[key] = p
		}
		return p
	}
	getQuota := func(ns, name, res string) *ResourceQuotaMetrics {
		key := ns + "/" + name + "/" + res
		q, ok := quotas[key]
		if !ok {
			q = &ResourceQuotaMetrics{Namespace: ns, Name: name, Resource: res}
			quotas[key] = q
		}
		return q
	}
	getCronJob := func(ns, name string) *CronJobMetrics {
		key := ns + "/" + name
		cj, ok := cronJobs[key]
		if !ok {
			cj = &CronJobMetrics{Namespace: ns, Name: name}
			cronJobs[key] = cj
		}
		return cj
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}

		metricName, rawLabels, val, ok := parsePromLine(line)
		if !ok {
			continue
		}
		if _, allowed := allowlistedMetrics[metricName]; !allowed {
			continue
		}

		lbls := parsePromLabels(rawLabels)
		ns := lbls["namespace"]

		switch metricName {
		case "kube_horizontalpodautoscaler_info":
			name := lbls["horizontalpodautoscaler"]
			if name == "" {
				continue
			}
			h := getHPA(ns, name)
			if k := firstNonEmpty(lbls["scaletargetref_kind"], lbls["scale_target_ref_kind"], lbls["target_kind"]); k != "" {
				h.TargetKind = k
			}
			if n := firstNonEmpty(lbls["scaletargetref_name"], lbls["scale_target_ref_name"], lbls["target_name"]); n != "" {
				h.TargetName = n
			}

		case "kube_horizontalpodautoscaler_spec_min_replicas":
			if name := lbls["horizontalpodautoscaler"]; name != "" {
				getHPA(ns, name).MinReplicas = int32(math.Round(val))
			}

		case "kube_horizontalpodautoscaler_spec_max_replicas":
			if name := lbls["horizontalpodautoscaler"]; name != "" {
				getHPA(ns, name).MaxReplicas = int32(math.Round(val))
			}

		case "kube_horizontalpodautoscaler_status_current_replicas":
			if name := lbls["horizontalpodautoscaler"]; name != "" {
				getHPA(ns, name).CurrentReplicas = int32(math.Round(val))
			}

		case "kube_horizontalpodautoscaler_status_desired_replicas":
			if name := lbls["horizontalpodautoscaler"]; name != "" {
				getHPA(ns, name).DesiredReplicas = int32(math.Round(val))
			}

		case "kube_horizontalpodautoscaler_status_condition":
			name := lbls["horizontalpodautoscaler"]
			if name == "" || val == 0 {
				continue
			}
			status := strings.ToLower(lbls["status"])
			if status != "true" && status != "false" {
				continue
			}
			bVal := status == "true"
			h := getHPA(ns, name)
			switch lbls["condition"] {
			case "AbleToScale":
				h.AbleToScale = &bVal
			case "ScalingActive":
				h.ScalingActive = &bVal
			case "ScalingLimited":
				h.ScalingLimited = &bVal
			}

		case "kube_poddisruptionbudget_status_current_healthy":
			if name := lbls["poddisruptionbudget"]; name != "" {
				getPDB(ns, name).CurrentHealthy = int32(math.Round(val))
			}

		case "kube_poddisruptionbudget_status_desired_healthy":
			if name := lbls["poddisruptionbudget"]; name != "" {
				getPDB(ns, name).DesiredHealthy = int32(math.Round(val))
			}

		case "kube_poddisruptionbudget_status_expected_pods":
			if name := lbls["poddisruptionbudget"]; name != "" {
				getPDB(ns, name).ExpectedPods = int32(math.Round(val))
			}

		case "kube_poddisruptionbudget_status_pod_disruptions_allowed":
			if name := lbls["poddisruptionbudget"]; name != "" {
				getPDB(ns, name).DisruptionsAllowed = int32(math.Round(val))
			}

		case "kube_resourcequota":
			name := lbls["resourcequota"]
			res := lbls["resource"]
			qType := lbls["type"]
			if name == "" || res == "" {
				continue
			}
			q := getQuota(ns, name, res)
			switch qType {
			case "hard":
				q.Hard = val
			case "used":
				q.Used = val
			}

		case "kube_cronjob_spec_suspend":
			if name := lbls["cronjob"]; name != "" {
				getCronJob(ns, name).Suspended = val != 0
			}

		case "kube_cronjob_status_active":
			if name := lbls["cronjob"]; name != "" {
				getCronJob(ns, name).Active = int32(math.Round(val))
			}

		case "kube_cronjob_status_last_schedule_time":
			if name := lbls["cronjob"]; name != "" && val > 0 {
				getCronJob(ns, name).LastScheduleTime = time.Unix(int64(val), 0).UTC()
			}

		case "kube_cronjob_next_schedule_time":
			if name := lbls["cronjob"]; name != "" && val > 0 {
				getCronJob(ns, name).NextScheduleTime = time.Unix(int64(val), 0).UTC()
			}

		case "kube_pod_status_disruption_reason":
			pod := lbls["pod"]
			reason := lbls["reason"]
			if pod != "" && reason != "" && val != 0 {
				disruptions[ns+"/"+pod] = reason
			}

		case "kube_pod_resourceclaim_info":
			pod := lbls["pod"]
			if pod == "" || val == 0 {
				continue
			}
			ref := ResourceClaimRef{
				Name:              lbls["name"],
				ResourceClaimName: firstNonEmpty(lbls["resourceclaim_name"], lbls["resource_claim_name"]),
				TemplateName:      firstNonEmpty(lbls["resourceclaimtemplate_name"], lbls["resource_claim_template_name"]),
			}
			dedupKey := ns + "/" + pod + "/" + ref.Name + "/" + ref.ResourceClaimName + "/" + ref.TemplateName
			if _, exists := claimSeen[dedupKey]; exists {
				continue
			}
			claimSeen[dedupKey] = struct{}{}
			claims[ns+"/"+pod] = append(claims[ns+"/"+pod], ref)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	snap := &Snapshot{
		Available:         true,
		Endpoint:          endpoint,
		ScrapedAt:         now.UTC(),
		HPAs:              make([]HPAMetrics, 0, len(hpas)),
		PDBs:              make([]PDBMetrics, 0, len(pdbs)),
		ResourceQuotas:    make([]ResourceQuotaMetrics, 0, len(quotas)),
		CronJobs:          make([]CronJobMetrics, 0, len(cronJobs)),
		PodDisruptions:    disruptions,
		PodResourceClaims: claims,
	}

	for _, h := range hpas {
		snap.HPAs = append(snap.HPAs, *h)
	}
	sort.Slice(snap.HPAs, func(i, j int) bool {
		if snap.HPAs[i].Namespace != snap.HPAs[j].Namespace {
			return snap.HPAs[i].Namespace < snap.HPAs[j].Namespace
		}
		return snap.HPAs[i].Name < snap.HPAs[j].Name
	})

	for _, p := range pdbs {
		snap.PDBs = append(snap.PDBs, *p)
	}
	sort.Slice(snap.PDBs, func(i, j int) bool {
		if snap.PDBs[i].Namespace != snap.PDBs[j].Namespace {
			return snap.PDBs[i].Namespace < snap.PDBs[j].Namespace
		}
		return snap.PDBs[i].Name < snap.PDBs[j].Name
	})

	for _, q := range quotas {
		if q.Hard > 0 {
			q.UsedPct = math.Round((q.Used/q.Hard)*1000) / 10
		}
		snap.ResourceQuotas = append(snap.ResourceQuotas, *q)
	}
	sort.Slice(snap.ResourceQuotas, func(i, j int) bool {
		if snap.ResourceQuotas[i].Namespace != snap.ResourceQuotas[j].Namespace {
			return snap.ResourceQuotas[i].Namespace < snap.ResourceQuotas[j].Namespace
		}
		if snap.ResourceQuotas[i].Name != snap.ResourceQuotas[j].Name {
			return snap.ResourceQuotas[i].Name < snap.ResourceQuotas[j].Name
		}
		return snap.ResourceQuotas[i].Resource < snap.ResourceQuotas[j].Resource
	})

	for _, cj := range cronJobs {
		snap.CronJobs = append(snap.CronJobs, *cj)
	}
	sort.Slice(snap.CronJobs, func(i, j int) bool {
		if snap.CronJobs[i].Namespace != snap.CronJobs[j].Namespace {
			return snap.CronJobs[i].Namespace < snap.CronJobs[j].Namespace
		}
		return snap.CronJobs[i].Name < snap.CronJobs[j].Name
	})

	return snap, nil
}

// parsePromLine splits a Prometheus text sample line into metric name, raw label content,
// and numeric float64 value without allocating intermediate label maps until allowlisted.
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

// parsePromLabels extracts key="value" pairs from inside a Prometheus label block.
func parsePromLabels(raw string) map[string]string {
	out := make(map[string]string, 6)
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
		i++ // skip opening quote
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
