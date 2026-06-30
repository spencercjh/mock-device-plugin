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
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newAscendNode builds a node carrying the hami.io/node-register-<commonWord> annotation
// and a count-resource capacity used by CheckHealthy.
func newAscendNode(commonWord, resourceName, annoJSON, capacity string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				"hami.io/node-register-" + commonWord: annoJSON,
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceName(resourceName): resource.MustParse(capacity),
			},
		},
	}
}

// cards builds a JSON device list with n identical cards (devmem/devcore per card).
func cards(n int, devmem, devcore int) string {
	items := make([]string, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, fmt.Sprintf(`{"id":"MOCK-%d","index":%d,"devmem":%d,"devcore":%d,"type":"Ascend910B4","health":true}`, i, i, devmem, devcore))
	}
	return "[" + strings.Join(items, ",") + "]"
}

func newDev(coreName string, aiCore, memFactor int32) *Devices {
	return &Devices{
		config: VNPUConfig{
			CommonWord:         "Ascend910B4",
			ResourceName:       "huawei.com/Ascend910B4",
			ResourceMemoryName: "huawei.com/Ascend910B4-memory",
			ResourceCoreName:   coreName,
			AICore:             aiCore,
			MemoryFactor:       memFactor,
		},
		nodeRegisterAnno: "hami.io/node-register-Ascend910B4",
	}
}

// TestGetResource_Core covers core-resource registration.
func TestGetResource_Core(t *testing.T) {
	const (
		memName  = "Ascend910B4-memory"
		coreName = "Ascend910B4-core"
		coreFull = "huawei.com/Ascend910B4-core"
	)
	tests := []struct {
		name        string
		dev         *Devices
		anno        string
		capacity    string
		wantMem     int
		wantCore    int
		wantCoreKey bool
	}{
		{
			name:        "1 core + devcore in annotation",
			dev:         newDev(coreFull, 20, 0),
			anno:        cards(2, 32768, 20),
			capacity:    "8",
			wantMem:     65536,
			wantCore:    40,
			wantCoreKey: true,
		},
		{
			name:        "2 183 real scale (8 cards)",
			dev:         newDev(coreFull, 20, 0),
			anno:        cards(8, 32768, 20),
			capacity:    "32",
			wantMem:     262144,
			wantCore:    160,
			wantCoreKey: true,
		},
		{
			name:        "3 annotation missing devcore -> fallback to config.AICore",
			dev:         newDev(coreFull, 20, 0),
			anno:        cards(2, 21527, 0),
			capacity:    "8",
			wantMem:     43054,
			wantCore:    40, // 2 * config.AICore(20)
			wantCoreKey: true,
		},
		{
			name:        "4 no resourceCoreName -> memory only (backward compat)",
			dev:         newDev("", 20, 0),
			anno:        cards(2, 32768, 20),
			capacity:    "8",
			wantMem:     65536,
			wantCore:    0,
			wantCoreKey: false,
		},
		{
			name:        "5 MemoryFactor does not affect core",
			dev:         newDev(coreFull, 20, 2),
			anno:        cards(2, 16384, 20),
			capacity:    "8",
			wantMem:     16384, // (16384*2)/2
			wantCore:    40,
			wantCoreKey: true,
		},
		{
			name:        "6 unhealthy node (capacity 0)",
			dev:         newDev(coreFull, 20, 0),
			anno:        cards(2, 32768, 20),
			capacity:    "0",
			wantMem:     0,
			wantCore:    0,
			wantCoreKey: true, // key present but zero
		},
		{
			name:        "7 mixed devcore (one card 0 -> fallback)",
			dev:         newDev(coreFull, 20, 0),
			anno:        `[{"id":"A","devmem":32768,"devcore":20,"type":"Ascend910B4","health":true},{"id":"B","index":1,"devmem":32768,"devcore":0,"type":"Ascend910B4","health":true}]`,
			capacity:    "8",
			wantMem:     65536,
			wantCore:    40, // 20 + fallback 20
			wantCoreKey: true,
		},
		{
			name:        "8 no register annotation",
			dev:         newDev(coreFull, 20, 0),
			anno:        "", // overwritten below to remove annotation
			capacity:    "8",
			wantMem:     0,
			wantCore:    0,
			wantCoreKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := newAscendNode("Ascend910B4", "huawei.com/Ascend910B4", tt.anno, tt.capacity)
			if tt.anno == "" {
				// case 8: healthy node but no register annotation at all
				delete(node.Annotations, "hami.io/node-register-Ascend910B4")
			}
			res := node // alias
			got := tt.dev.GetResource(res)

			if got[memName] != tt.wantMem {
				t.Errorf("memory = %d, want %d", got[memName], tt.wantMem)
			}
			_, hasCore := got[coreName]
			if hasCore != tt.wantCoreKey {
				t.Errorf("core key present = %v, want %v (map=%v)", hasCore, tt.wantCoreKey, got)
			}
			if tt.wantCoreKey && got[coreName] != tt.wantCore {
				t.Errorf("core = %d, want %d", got[coreName], tt.wantCore)
			}
		})
	}
}

// TestGetNodeDevices_RealAnnotation feeds a real 183 node-register annotation slice
// (2 cards, including index omitempty / custominfo / devicepairscore) to ensure the
// mock DeviceInfo can losslessly decode what the real ascend-device-plugin reports
// A tag drift that drops devcore would make the core resource 0.
func TestGetNodeDevices_RealAnnotation(t *testing.T) {
	const realAnno = `[{"id":"7D16F664-806034F3-5BC13B72-D0D00485-104301E3","count":4,"devmem":32768,"devcore":20,"type":"Ascend910B4","health":true,"custominfo":{"NetworkID":0},"devicepairscore":{}},{"id":"16DFF664-806050DD-5CE21592-87D28485-104301E3","index":1,"count":4,"devmem":32768,"devcore":20,"type":"Ascend910B4","health":true,"custominfo":{"NetworkID":0},"devicepairscore":{}}]`

	dev := newDev("huawei.com/Ascend910B4-core", 20, 0)
	node := newAscendNode("Ascend910B4", "huawei.com/Ascend910B4", realAnno, "32")

	devs, err := dev.GetNodeDevices(node)
	if err != nil {
		t.Fatalf("GetNodeDevices error: %v", err)
	}
	if len(devs) != 2 {
		t.Fatalf("got %d devices, want 2", len(devs))
	}
	for i, d := range devs {
		if d.Devmem != 32768 {
			t.Errorf("dev[%d].Devmem = %d, want 32768", i, d.Devmem)
		}
		if d.Devcore != 20 {
			t.Errorf("dev[%d].Devcore = %d, want 20", i, d.Devcore)
		}
		if d.Count != 4 {
			t.Errorf("dev[%d].Count = %d, want 4", i, d.Count)
		}
		if d.Type != "Ascend910B4" {
			t.Errorf("dev[%d].Type = %q, want Ascend910B4", i, d.Type)
		}
		if !d.Health {
			t.Errorf("dev[%d].Health = false, want true", i)
		}
		if d.DeviceVendor != "Ascend910B4" {
			t.Errorf("dev[%d].DeviceVendor = %q, want Ascend910B4", i, d.DeviceVendor)
		}
	}
	if devs[0].Index != 0 {
		t.Errorf("dev[0].Index = %d, want 0 (omitempty)", devs[0].Index)
	}
	if devs[1].Index != 1 {
		t.Errorf("dev[1].Index = %d, want 1", devs[1].Index)
	}
}
