package hls

import (
	"slices"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// displayAdapters is the registry key of Windows's display adapters, one
// subkey each, 0000 and on.
const displayAdapters = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`

// vendors are the names the adapters of a GPU method's vendor carry.
var vendors = map[string][]string{"cuda": {"NVIDIA"}, "qsv": {"Intel"}, "amf": {"AMD", "Advanced Micro Devices"}}

// systemGPU names the GPU of hw and its driver as Windows tells them: the
// description and driver version of the display adapters of hw's vendor,
// of every adapter when none matches. Empty for what it cannot read.
func (m *Manager) systemGPU(hw *Hardware) (gpu, driver string) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, displayAdapters, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return "", ""
	}
	defer key.Close()
	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return "", ""
	}
	slices.Sort(names)
	var all, matching [][2]string
	for _, name := range names {
		adapter, err := registry.OpenKey(key, name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		description, _, _ := adapter.GetStringValue("DriverDesc")
		version, _, _ := adapter.GetStringValue("DriverVersion")
		provider, _, _ := adapter.GetStringValue("ProviderName")
		adapter.Close()
		if description == "" {
			continue
		}
		all = append(all, [2]string{description, version})
		if slices.ContainsFunc(vendors[hw.Method], func(vendor string) bool {
			return strings.Contains(description, vendor) || strings.Contains(provider, vendor)
		}) {
			matching = append(matching, [2]string{description, version})
		}
	}
	if len(matching) == 0 {
		matching = all
	}
	var gpus, drivers []string
	for _, adapter := range matching {
		gpus, drivers = append(gpus, adapter[0]), append(drivers, adapter[1])
	}
	return strings.Join(gpus, ", "), strings.Join(drivers, ", ")
}
