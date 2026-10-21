package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const recordVersion = 1

var (
	safeVersionPattern   = regexp.MustCompile(`^[vV]?[0-9]+(?:\.[0-9]+){1,3}$`)
	kernelVersionPattern = regexp.MustCompile(`^\s*([0-9]+)\.([0-9]+)`)
	engineVersionPattern = regexp.MustCompile(`(?i)(?:docker\s+version\s+)?v?([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)
)

type Collector struct {
	api                   *APIClient
	includeInstallationID bool
	now                   func() time.Time
}

type installationStats struct {
	UID     string         `json:"uid"`
	Image   string         `json:"image"`
	Version string         `json:"version"`
	Auth    map[string]int `json:"auth"`
}

type environmentStats struct {
	Total         int            `json:"total"`
	Orchestration map[string]int `json:"orch"`
}

type cpuStats struct {
	CoresMin   int `json:"cores_min"`
	CoresMax   int `json:"cores_max"`
	CoresTotal int `json:"cores_total"`
	MHzTotal   int `json:"mhz_total"`
	UtilMin    int `json:"util_min"`
	UtilAvg    int `json:"util_avg"`
	UtilMax    int `json:"util_max"`
}

type memoryStats struct {
	MinMB   int `json:"mb_min"`
	MaxMB   int `json:"mb_max"`
	TotalMB int `json:"mb_total"`
	UtilMin int `json:"util_min"`
	UtilAvg int `json:"util_avg"`
	UtilMax int `json:"util_max"`
}

type hostStats struct {
	Active     int            `json:"active"`
	Total      int            `json:"total"`
	CPU        cpuStats       `json:"cpu"`
	Memory     memoryStats    `json:"mem"`
	Kernel     map[string]int `json:"kernel"`
	OS         map[string]int `json:"os"`
	DockerFull map[string]int `json:"docker_full"`
	Docker     map[string]int `json:"docker"`
	Driver     map[string]int `json:"driver"`
}

type containerStats struct {
	Running    int `json:"running"`
	Total      int `json:"total"`
	PerHostMin int `json:"per_host_min"`
	PerHostAvg int `json:"per_host_avg"`
	PerHostMax int `json:"per_host_max"`
}

type serviceStats struct {
	Active      int            `json:"active"`
	Total       int            `json:"total"`
	Kind        map[string]int `json:"kind"`
	PerStackMin int            `json:"per_stack_min"`
	PerStackAvg int            `json:"per_stack_avg"`
	PerStackMax int            `json:"per_stack_max"`
}

type stackStats struct {
	Active      int `json:"active"`
	FromCatalog int `json:"from_catalog"`
	Total       int `json:"total"`
	PerEnvMin   int `json:"per_env_min"`
	PerEnvAvg   int `json:"per_env_avg"`
	PerEnvMax   int `json:"per_env_max"`
}

func NewCollector(api *APIClient, includeInstallationID bool) *Collector {
	return &Collector{api: api, includeInstallationID: includeInstallationID, now: time.Now}
}

func (c *Collector) Collect(ctx context.Context) (map[string]any, error) {
	record := map[string]any{
		"r":  recordVersion,
		"ts": c.now().UTC().Format(time.RFC3339),
	}
	partial := make([]string, 0)
	successes := 0

	install, ok := c.collectInstallation(ctx)
	record["install"] = install
	if ok {
		successes++
	} else {
		partial = append(partial, "install")
	}

	projects, err := c.api.List(ctx, "projects", url.Values{"state_ne": []string{"removed"}, "all": []string{"true"}})
	if err != nil {
		partial = append(partial, "environment")
		record["environment"] = environmentStats{Orchestration: map[string]int{}}
	} else {
		successes++
		record["environment"] = summarizeEnvironments(projects)
	}

	hosts, err := c.api.List(ctx, "hosts", url.Values{"state_ne": []string{"removed"}})
	if err != nil {
		partial = append(partial, "host")
		record["host"] = emptyHostStats()
	} else {
		successes++
		machines, machineErr := c.api.List(ctx, "machines", url.Values{"state_ne": []string{"removed"}})
		if machineErr != nil {
			partial = append(partial, "machine-driver")
			machines = nil
		} else {
			successes++
		}
		record["host"] = summarizeHosts(hosts, machines)
	}

	containers, err := c.api.List(ctx, "containers", url.Values{"state_ne": []string{"removed"}})
	if err != nil {
		partial = append(partial, "container")
		record["container"] = containerStats{}
	} else {
		successes++
		record["container"] = summarizeContainers(containers)
	}

	services, err := c.api.List(ctx, "services", url.Values{"state_ne": []string{"removed"}})
	if err != nil {
		partial = append(partial, "service")
		record["service"] = serviceStats{Kind: map[string]int{}}
	} else {
		successes++
		record["service"] = summarizeServices(services)
	}

	stacks, err := c.api.List(ctx, "stacks", url.Values{"state_ne": []string{"removed"}})
	if err != nil {
		partial = append(partial, "stack")
		record["stack"] = stackStats{}
	} else {
		successes++
		record["stack"] = summarizeStacks(stacks)
	}

	sort.Strings(partial)
	record["_meta"] = map[string]any{
		"partial":                 partial,
		"raw_names_collected":     false,
		"raw_addresses_collected": false,
		"raw_image_coordinates":   false,
	}
	if successes == 0 {
		return record, errors.New("compatible API queries failed")
	}
	return record, nil
}

func (c *Collector) collectInstallation(ctx context.Context) (installationStats, bool) {
	out := installationStats{UID: "not-collected", Image: "unknown", Version: "unknown", Auth: map[string]int{}}
	success := false
	setting := func(name string) string {
		value, err := c.api.Setting(ctx, name)
		if err == nil {
			success = true
		}
		return value
	}

	if c.includeInstallationID {
		if uid := setting("telemetry.uid"); uid != "" {
			sum := sha256.Sum256([]byte(uid))
			out.UID = "sha256:" + hex.EncodeToString(sum[:])
		}
	}
	// These setting names are inherited wire-compatibility identifiers. Raw
	// image coordinates are deliberately reduced to a non-identifying class.
	out.Image = classifyImage(setting("rancher.server.image"))
	out.Version = safeVersion(setting("rancher.server.version"))
	auth := "none"
	if strings.EqualFold(setting("api.security.enabled"), "true") {
		provider := strings.TrimSuffix(strings.ToLower(setting("api.auth.provider.configured")), "config")
		auth = normalizeAuthProvider(provider)
	}
	increment(out.Auth, auth)
	return out, success
}

func summarizeEnvironments(items []map[string]any) environmentStats {
	out := environmentStats{Total: len(items), Orchestration: map[string]int{}}
	for _, item := range items {
		value := stringValue(item["orchestration"])
		if value == "" {
			value = "native"
		}
		increment(out.Orchestration, normalizeOrchestration(value))
	}
	return out
}

func emptyHostStats() hostStats {
	return hostStats{Kernel: map[string]int{}, OS: map[string]int{}, DockerFull: map[string]int{}, Docker: map[string]int{}, Driver: map[string]int{}}
}

func summarizeHosts(hosts, machines []map[string]any) hostStats {
	out := emptyHostStats()
	var cpuUtils, memoryUtils []float64
	for _, host := range hosts {
		out.Total++
		if stringValue(host["state"]) == "active" {
			out.Active++
		}
		info, ok := mapValue(host["info"])
		if !ok {
			continue
		}
		if cpu, ok := mapValue(info["cpuInfo"]); ok {
			cores := roundedInt(cpu["count"])
			mhz := roundedInt(cpu["mhz"])
			out.CPU.CoresMin = minNonZero(out.CPU.CoresMin, cores)
			out.CPU.CoresMax = maxInt(out.CPU.CoresMax, cores)
			out.CPU.CoresTotal += cores
			out.CPU.MHzTotal += mhz
			if percentages, ok := sliceValue(cpu["cpuCoresPercentages"]); ok && len(percentages) > 0 {
				var total float64
				for _, value := range percentages {
					total += numberValue(value)
				}
				util := total / float64(len(percentages))
				cpuUtils = append(cpuUtils, util)
				out.CPU.UtilMin = minNonZero(out.CPU.UtilMin, int(math.Round(util)))
				out.CPU.UtilMax = maxInt(out.CPU.UtilMax, int(math.Round(util)))
			}
		}
		if memory, ok := mapValue(info["memoryInfo"]); ok {
			total := roundedInt(memory["memTotal"])
			available := roundedInt(memory["memAvailable"])
			if total > 0 {
				util := 100 * float64(clampInt(0, total-available, total)) / float64(total)
				memoryUtils = append(memoryUtils, util)
				out.Memory.MinMB = minNonZero(out.Memory.MinMB, total)
				out.Memory.MaxMB = maxInt(out.Memory.MaxMB, total)
				out.Memory.TotalMB += total
				out.Memory.UtilMin = minNonZero(out.Memory.UtilMin, int(math.Round(util)))
				out.Memory.UtilMax = maxInt(out.Memory.UtilMax, int(math.Round(util)))
			}
		}
		if osInfo, ok := mapValue(info["osInfo"]); ok {
			increment(out.Kernel, normalizeKernelVersion(stringValue(osInfo["kernelVersion"])))
			increment(out.OS, classifyOperatingSystem(stringValue(osInfo["operatingSystem"])))
			docker := normalizeDockerVersion(stringValue(osInfo["dockerVersion"]))
			increment(out.Docker, docker)
			increment(out.DockerFull, docker)
		}
	}
	out.CPU.UtilAvg = clampInt(0, averageRounded(cpuUtils), 100)
	out.Memory.UtilAvg = clampInt(0, averageRounded(memoryUtils), 100)
	for _, machine := range machines {
		increment(out.Driver, normalizeMachineDriver(stringValue(machine["driver"])))
	}
	return out
}

func summarizeContainers(items []map[string]any) containerStats {
	out := containerStats{Total: len(items)}
	byHost := map[string]int{}
	for _, item := range items {
		state := stringValue(item["state"])
		if state == "running" || state == "started-once" {
			out.Running++
		}
		byHost[stringValue(item["hostId"])]++
	}
	out.PerHostMin, out.PerHostAvg, out.PerHostMax = distribution(byHost)
	return out
}

func summarizeServices(items []map[string]any) serviceStats {
	out := serviceStats{Total: len(items), Kind: map[string]int{}}
	byStack := map[string]int{}
	for _, item := range items {
		if stringValue(item["state"]) == "active" {
			out.Active++
		}
		byStack[stringValue(item["stackId"])]++
		increment(out.Kind, normalizeServiceKind(stringValue(item["type"])))
	}
	out.PerStackMin, out.PerStackAvg, out.PerStackMax = distribution(byStack)
	return out
}

func summarizeStacks(items []map[string]any) stackStats {
	out := stackStats{Total: len(items)}
	byEnvironment := map[string]int{}
	for _, item := range items {
		if stringValue(item["state"]) == "active" {
			out.Active++
		}
		if strings.Contains(stringValue(item["externalId"]), "catalog://") {
			out.FromCatalog++
		}
		byEnvironment[stringValue(item["accountId"])]++
	}
	out.PerEnvMin, out.PerEnvAvg, out.PerEnvMax = distribution(byEnvironment)
	return out
}

func classifyImage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	if strings.Contains(value, "pasturestack") {
		return "pasturestack"
	}
	return "custom"
}

func safeVersion(value string) string {
	value = strings.TrimSpace(value)
	if safeVersionPattern.MatchString(value) && len(value) <= 64 {
		return value
	}
	if value == "" {
		return "unknown"
	}
	return "custom"
}

func normalizeDockerVersion(value string) string {
	match := engineVersionPattern.FindStringSubmatch(value)
	if len(match) != 2 {
		if strings.TrimSpace(value) == "" {
			return "unknown"
		}
		return "other"
	}
	return "v" + match[1]
}

func normalizeKernelVersion(value string) string {
	match := kernelVersionPattern.FindStringSubmatch(value)
	if len(match) != 3 {
		if strings.TrimSpace(value) == "" {
			return "unknown"
		}
		return "other"
	}
	return match[1] + "." + match[2]
}

func classifyOperatingSystem(value string) string {
	normalized := strings.ToLower(value)
	for _, candidate := range []struct {
		key   string
		label string
	}{
		{"ubuntu", "ubuntu"},
		{"debian", "debian"},
		{"red hat", "red-hat"},
		{"rhel", "red-hat"},
		{"centos", "centos"},
		{"fedora", "fedora"},
		{"rocky", "rocky-linux"},
		{"alma", "almalinux"},
		{"alpine", "alpine-linux"},
		{"suse", "suse-linux"},
		{"sles", "suse-linux"},
		{"coreos", "coreos"},
		{"photon", "photon-os"},
		{"windows", "windows"},
	} {
		if strings.Contains(normalized, candidate.key) {
			return candidate.label
		}
	}
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return "other"
}

func normalizeOrchestration(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "cattle", "native":
		return "native"
	case "kubernetes":
		return "kubernetes"
	case "swarm":
		return "swarm"
	case "mesos":
		return "mesos"
	default:
		return "other"
	}
}

func normalizeAuthProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "":
		return "none"
	case "local", "localauth":
		return "local"
	case "ldap", "openldap":
		return "ldap"
	case "activedirectory", "ad":
		return "active-directory"
	case "github":
		return "github"
	case "azuread":
		return "azure-ad"
	case "shibboleth":
		return "shibboleth"
	default:
		return "other"
	}
}

func normalizeMachineDriver(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "generic", "amazonec2", "azure", "digitalocean", "exoscale", "google", "openstack", "packet", "rackspace", "softlayer", "virtualbox", "vmwarefusion", "vmwarevsphere":
		return strings.ToLower(strings.TrimSpace(value))
	case "":
		return "unknown"
	default:
		return "other"
	}
}

func normalizeServiceKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "service":
		return "service"
	case "loadbalancerservice":
		return "load-balancer-service"
	case "externalservice":
		return "external-service"
	case "dnsservice":
		return "dns-service"
	case "kubernetesservice":
		return "kubernetes-service"
	case "":
		return "unknown"
	default:
		return "other"
	}
}

func increment(values map[string]int, key string) {
	if key == "" {
		key = "unknown"
	}
	values[key]++
}

func distribution(values map[string]int) (int, int, int) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	minValue, maxValue, total := 0, 0, 0
	for _, value := range values {
		minValue = minNonZero(minValue, value)
		maxValue = maxInt(maxValue, value)
		total += value
	}
	return minValue, int(math.Round(float64(total) / float64(len(values)))), maxValue
}

func averageRounded(values []float64) int {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return int(math.Round(total / float64(len(values))))
}

func minNonZero(current, value int) int {
	if value <= 0 {
		return current
	}
	if current == 0 || value < current {
		return value
	}
	return current
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func clampInt(minimum, value, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func roundedInt(value any) int {
	return int(math.Round(numberValue(value)))
}

func mapValue(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}

func sliceValue(value any) ([]any, bool) {
	result, ok := value.([]any)
	return result, ok
}
