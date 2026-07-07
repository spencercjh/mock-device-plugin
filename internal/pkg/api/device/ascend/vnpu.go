/*
Copyright 2026 The HAMi Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ascend

import "k8s.io/klog/v2"

// VNPUs holds the global Ascend vNPU configuration. It mirrors the upstream HAMi
// structure (pkg/device/ascend/vnpu.go): a global hami-vnpu-core switch plus the
// per-chip config list under `configs`.
//
// Upstream HAMi switched the `vnpus` config from a bare list ([]VNPUConfig) to this
// nested object (since v2.9.0). To avoid breaking users who have not upgraded their
// device-config ConfigMap (e.g. downstream/commercial deployments still on the old
// flat-list format), UnmarshalYAML accepts BOTH formats.
type VNPUs struct {
	HamiVnpuCore bool         `yaml:"hamiVnpuCore"`
	Configs      []VNPUConfig `yaml:"configs"`
}

// UnmarshalYAML implements dual-format parsing for the `vnpus` field:
//  1. Prefer the new nested object: {hamiVnpuCore: bool, configs: [...]}.
//  2. Fall back to the legacy flat list: [VNPUConfig, ...].
//  3. If neither matches, return the error from the preferred (new) format so the
//     failure surfaces with the original semantics.
func (v *VNPUs) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// 1) Preferred: new nested structure. Use an alias type to avoid recursing
	// into this very UnmarshalYAML method.
	type vnpusAlias VNPUs
	var nested vnpusAlias
	errNew := unmarshal(&nested)
	if errNew == nil {
		*v = VNPUs(nested)
		return nil
	}

	// 2) Fallback: legacy flat list of per-chip configs.
	var list []VNPUConfig
	if errOld := unmarshal(&list); errOld == nil {
		v.HamiVnpuCore = false
		v.Configs = list
		klog.Warning("vnpus parsed as legacy flat-list format; consider upgrading the device-config to {hamiVnpuCore, configs: [...]}")
		return nil
	}

	// 3) Neither format matched: report the preferred-format error.
	return errNew
}
