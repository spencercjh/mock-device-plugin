/*
Copyright 2025 The HAMi Authors.

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

import (
	"testing"

	"gopkg.in/yaml.v2"
)

// wrapper mirrors how config.Config embeds VNPUs under the top-level `vnpus` key,
// so the test exercises the same code path as the real config parsing.
type wrapper struct {
	VNPUs VNPUs `yaml:"vnpus"`
}

// TestVNPUs_UnmarshalYAML_DualFormat verifies the backward-compatible parsing:
// new nested format is preferred, the legacy flat-list format is accepted as a
// fallback, and anything else surfaces an error.
func TestVNPUs_UnmarshalYAML_DualFormat(t *testing.T) {
	tests := []struct {
		name             string
		yamlStr          string
		wantErr          bool
		wantHamiVnpuCore bool
		wantConfigs      int
		wantFirstCommon  string
		wantFirstCore    string
	}{
		{
			name: "new nested format",
			yamlStr: `
vnpus:
  hamiVnpuCore: true
  configs:
  - chipName: 910B4
    commonWord: Ascend910B4
    resourceName: huawei.com/Ascend910B4
    resourceMemoryName: huawei.com/Ascend910B4-memory
    resourceCoreName: huawei.com/Ascend910B4-core
    aiCore: 20
    templates:
    - name: vir05_1c_8g
      memory: 8192
      aiCore: 5
`,
			wantErr:          false,
			wantHamiVnpuCore: true,
			wantConfigs:      1,
			wantFirstCommon:  "Ascend910B4",
			wantFirstCore:    "huawei.com/Ascend910B4-core",
		},
		{
			name: "legacy flat-list format",
			yamlStr: `
vnpus:
- chipName: 910B4
  commonWord: Ascend910B4
  resourceName: huawei.com/Ascend910B4
  resourceMemoryName: huawei.com/Ascend910B4-memory
  aiCore: 20
`,
			wantErr:          false,
			wantHamiVnpuCore: false,
			wantConfigs:      1,
			wantFirstCommon:  "Ascend910B4",
			wantFirstCore:    "",
		},
		{
			name:        "invalid scalar",
			yamlStr:     "vnpus: \"not-a-list-or-map\"",
			wantErr:     true,
			wantConfigs: 0,
		},
		{
			name:        "empty/null vnpus",
			yamlStr:     "vnpus:",
			wantErr:     false,
			wantConfigs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w wrapper
			err := yaml.Unmarshal([]byte(tt.yamlStr), &w)
			if tt.wantErr != (err != nil) {
				t.Fatalf("unmarshal err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if w.VNPUs.HamiVnpuCore != tt.wantHamiVnpuCore {
				t.Errorf("HamiVnpuCore = %v, want %v", w.VNPUs.HamiVnpuCore, tt.wantHamiVnpuCore)
			}
			if len(w.VNPUs.Configs) != tt.wantConfigs {
				t.Fatalf("Configs len = %d, want %d", len(w.VNPUs.Configs), tt.wantConfigs)
			}
			if tt.wantConfigs > 0 {
				if w.VNPUs.Configs[0].CommonWord != tt.wantFirstCommon {
					t.Errorf("Configs[0].CommonWord = %q, want %q", w.VNPUs.Configs[0].CommonWord, tt.wantFirstCommon)
				}
				if w.VNPUs.Configs[0].ResourceCoreName != tt.wantFirstCore {
					t.Errorf("Configs[0].ResourceCoreName = %q, want %q", w.VNPUs.Configs[0].ResourceCoreName, tt.wantFirstCore)
				}
			}
		})
	}
}
