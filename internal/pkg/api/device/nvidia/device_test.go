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
package nvidia

import (
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func writeInventoryFile(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "mock-inventory.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write inventory: %v", err)
	}

	return path
}

func TestGetResource(t *testing.T) {

	// 创建 NVIDIA 设备配置
	config := NvidiaConfig{
		ResourceCountName:            "nvidia.com/gpu",
		ResourceMemoryName:           "nvidia.com/gpu-memory",
		ResourceCoreName:             "nvidia.com/gpu-core",
		ResourceMemoryPercentageName: "nvidia.com/gpu-memory-percentage",
		ResourcePriority:             "nvidia.com/gpu-priority",
		DefaultMemory:                0,
		DefaultCores:                 0,
		DefaultGPUNum:                1,
		OverwriteEnv:                 true,
		DisableCoreLimit:             false,
	}

	dev := InitNvidiaDevice(config, "")

	// 根据提供的 annotation 创建测试节点
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-nvidia-a100",
			Annotations: map[string]string{
				RegisterAnnos: `[
				{"id":"GPU-0","index":4,"count":10,"devmem":81920,"devcore":100,"type":"NVIDIA A100-SXM4-80GB","numa":1,"mode":"hami-core","health":true,"devicepairscore":{}},
				{"id":"GPU-1","index":5,"count":10,"devmem":81920,"devcore":100,"type":"NVIDIA A100-SXM4-80GB","numa":1,"mode":"hami-core","health":true,"devicepairscore":{}},
				{"id":"GPU-2","index":6,"count":10,"devmem":81920,"devcore":100,"type":"NVIDIA A100-SXM4-80GB","numa":1,"mode":"hami-core","health":true,"devicepairscore":{}}
				]`,
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceName(config.ResourceCountName): resource.MustParse("4"),
			},
		},
	}

	t.Run("Test Nvidia A100 device addition", func(t *testing.T) {
		result := dev.GetResource(&node)

		expectedMemoryResource := "gpu-memory"
		expectedCoreResource := "gpu-core"
		expectedMemoryPercentageResource := "gpu-memory-percentage"

		expectedTotalMemory := 245760
		actualMemory := result[expectedMemoryResource]
		if actualMemory != expectedTotalMemory {
			t.Errorf("expected total memory %d, got %d", expectedTotalMemory, actualMemory)
		}

		expectedTotalCore := 300
		actualCore := result[expectedCoreResource]
		if actualCore != expectedTotalCore {
			t.Errorf("expected total core %d, got %d", expectedTotalCore, actualCore)
		}

		expectedTotalMemoryPercentage := 300
		actualMemoryPercentage := result[expectedMemoryPercentageResource]
		if actualMemoryPercentage != expectedTotalMemoryPercentage {
			t.Errorf("expected total memory percentage %d, got %d", expectedTotalMemoryPercentage, actualMemoryPercentage)
		}

	})
}

func TestGetResourceUsesInventoryWithoutHealthGate(t *testing.T) {
	inventoryPath := writeInventoryFile(t, `
apiVersion: mock.hami.io/v1alpha1
kind: MockInventory
groupBy:
  labelKey: hami.io/mock-group
groups:
  gpu-a100:
    nvidia:
      - id: GPU-MOCK-0
        index: 0
        type: NVIDIA-A100-SXM4-80GB
        devmem: 81920
        devcore: 100
        count: 10
        health: true
`)

	config := NvidiaConfig{
		ResourceCountName:            "nvidia.com/gpu",
		ResourceMemoryName:           "nvidia.com/gpu-memory",
		ResourceCoreName:             "nvidia.com/gpu-core",
		ResourceMemoryPercentageName: "nvidia.com/gpu-memory-percentage",
	}
	dev := InitNvidiaDevice(config, inventoryPath)

	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a100-0",
			Labels: map[string]string{"hami.io/mock-group": "gpu-a100"},
		},
	}

	got := dev.GetResource(&node)
	if got["gpu-memory"] != 81920 {
		t.Fatalf("expected file-backed memory 81920, got %d", got["gpu-memory"])
	}
	if got["gpu-core"] != 100 {
		t.Fatalf("expected file-backed core 100, got %d", got["gpu-core"])
	}
	if got["gpu-memory-percentage"] != 100 {
		t.Fatalf("expected file-backed memory percentage 100, got %d", got["gpu-memory-percentage"])
	}
}

func TestGetResourceFallsBackToAnnotationWhenInventoryInactive(t *testing.T) {
	inventoryPath := writeInventoryFile(t, `
apiVersion: mock.hami.io/v1alpha1
kind: MockInventory
groupBy:
  labelKey: hami.io/mock-group
groups:
  gpu-a100: {}
`)

	config := NvidiaConfig{
		ResourceCountName:            "nvidia.com/gpu",
		ResourceMemoryName:           "nvidia.com/gpu-memory",
		ResourceCoreName:             "nvidia.com/gpu-core",
		ResourceMemoryPercentageName: "nvidia.com/gpu-memory-percentage",
	}
	dev := InitNvidiaDevice(config, inventoryPath)

	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-legacy",
			Labels: map[string]string{
				"hami.io/mock-group": "gpu-a100",
			},
			Annotations: map[string]string{
				RegisterAnnos: `[{"id":"GPU-0","index":0,"count":10,"devmem":40960,"devcore":50,"type":"NVIDIA-Legacy","health":true}]`,
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceName(config.ResourceCountName): resource.MustParse("1"),
			},
		},
	}

	got := dev.GetResource(&node)
	if got["gpu-memory"] != 40960 {
		t.Fatalf("expected annotation fallback memory 40960, got %d", got["gpu-memory"])
	}
	if got["gpu-core"] != 50 {
		t.Fatalf("expected annotation fallback core 50, got %d", got["gpu-core"])
	}
	if got["gpu-memory-percentage"] != 100 {
		t.Fatalf("expected annotation fallback memory percentage 100, got %d", got["gpu-memory-percentage"])
	}
}

func TestGetResourceFallsBackWhenOtherInventoryGroupIsInvalid(t *testing.T) {
	inventoryPath := writeInventoryFile(t, `
apiVersion: mock.hami.io/v1alpha1
kind: MockInventory
groupBy:
  labelKey: hami.io/mock-group
groups:
  gpu-a100:
    nvidia:
      - id: GPU-MOCK-0
        type: NVIDIA-A100-SXM4-80GB
        devmem: 81920
        devcore: 100
        count: 10
        health: true
`)

	config := NvidiaConfig{
		ResourceCountName:            "nvidia.com/gpu",
		ResourceMemoryName:           "nvidia.com/gpu-memory",
		ResourceCoreName:             "nvidia.com/gpu-core",
		ResourceMemoryPercentageName: "nvidia.com/gpu-memory-percentage",
	}
	dev := InitNvidiaDevice(config, inventoryPath)

	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-legacy",
			Labels: map[string]string{
				"hami.io/mock-group": "gpu-h100",
			},
			Annotations: map[string]string{
				RegisterAnnos: `[{"id":"GPU-0","index":0,"count":10,"devmem":40960,"devcore":50,"type":"NVIDIA-Legacy","health":true}]`,
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceName(config.ResourceCountName): resource.MustParse("1"),
			},
		},
	}

	got := dev.GetResource(&node)
	if got["gpu-memory"] != 40960 {
		t.Fatalf("expected annotation fallback memory 40960, got %d", got["gpu-memory"])
	}
	if got["gpu-core"] != 50 {
		t.Fatalf("expected annotation fallback core 50, got %d", got["gpu-core"])
	}
	if got["gpu-memory-percentage"] != 100 {
		t.Fatalf("expected annotation fallback memory percentage 100, got %d", got["gpu-memory-percentage"])
	}
}

func TestGetResourceSkipsInventoryNodeWhenPairScoreAnnotationMalformed(t *testing.T) {
	inventoryPath := writeInventoryFile(t, `
apiVersion: mock.hami.io/v1alpha1
kind: MockInventory
groupBy:
  labelKey: hami.io/mock-group
groups:
  gpu-a100:
    nvidia:
      - id: GPU-MOCK-0
        index: 0
        type: NVIDIA-A100-SXM4-80GB
        devmem: 81920
        devcore: 100
        count: 10
        health: true
`)

	config := NvidiaConfig{
		ResourceCountName:            "nvidia.com/gpu",
		ResourceMemoryName:           "nvidia.com/gpu-memory",
		ResourceCoreName:             "nvidia.com/gpu-core",
		ResourceMemoryPercentageName: "nvidia.com/gpu-memory-percentage",
	}
	dev := InitNvidiaDevice(config, inventoryPath)

	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a100-0",
			Labels: map[string]string{"hami.io/mock-group": "gpu-a100"},
			Annotations: map[string]string{
				RegisterGPUPairScore: "not-json",
			},
		},
	}

	got := dev.GetResource(&node)
	if got["gpu-memory"] != 0 {
		t.Fatalf("expected malformed pair-score annotation to skip inventory resources, got memory %d", got["gpu-memory"])
	}
	if got["gpu-core"] != 0 {
		t.Fatalf("expected malformed pair-score annotation to skip inventory resources, got core %d", got["gpu-core"])
	}
	if got["gpu-memory-percentage"] != 0 {
		t.Fatalf("expected malformed pair-score annotation to skip inventory resources, got memory percentage %d", got["gpu-memory-percentage"])
	}
}

func TestGetNodeDevices(t *testing.T) {
	tests := []struct {
		name        string
		setupNode   func() corev1.Node
		wantErr     bool
		wantDevices int
		setupDev    func() *NvidiaGPUDevices
	}{
		{
			name: "no annotation",
			setupNode: func() corev1.Node {
				return corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "node-no-anno",
						Annotations: map[string]string{},
					},
				}
			},
			wantErr:     true,
			wantDevices: 0,
			setupDev: func() *NvidiaGPUDevices {
				return &NvidiaGPUDevices{}
			},
		},
		{
			name: "invalid annotation format",
			setupNode: func() corev1.Node {
				return corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-bad-data",
						Annotations: map[string]string{
							RegisterAnnos: "invalid-data-format",
						},
					},
				}
			},
			wantErr:     true,
			wantDevices: 0,
			setupDev: func() *NvidiaGPUDevices {
				return &NvidiaGPUDevices{}
			},
		},
		{
			name: "empty devices annotation",
			setupNode: func() corev1.Node {
				return corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-empty-devices",
						Annotations: map[string]string{
							RegisterAnnos: "",
						},
					},
				}
			},
			wantErr:     true,
			wantDevices: 0,
			setupDev: func() *NvidiaGPUDevices {
				return &NvidiaGPUDevices{}
			},
		},
		{
			name: "old format",
			setupNode: func() corev1.Node {
				return corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-example",
						Annotations: map[string]string{
							RegisterAnnos: `GPU-f92d2cf4,10,81920,100,NVIDIA-NVIDIA A100-SXM4-80GB,1,true,6,hami-core:GPU-0d5a6e59,10,81920,100,NVIDIA-NVIDIA A100-SXM4-80GB,1,true,4,hami-core:GPU-da197561,10,81920,100,NVIDIA-NVIDIA A100-SXM4-80GB,1,true,5,hami-core:`,
						},
					},
				}
			},
			wantErr:     false,
			wantDevices: 3,
			setupDev: func() *NvidiaGPUDevices {
				return &NvidiaGPUDevices{
					config: NvidiaConfig{},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			node := tt.setupNode()
			dev := tt.setupDev()

			devices, err := dev.GetNodeDevices(&node)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetNodeDevices() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(devices) != tt.wantDevices {
				t.Errorf("GetNodeDevices() returned %d devices, want %d", len(devices), tt.wantDevices)
			}

			if !tt.wantErr && len(devices) > 0 {
				for _, d := range devices {
					if d.Devmem == 0 {
						t.Error("Devmem should not be zero")
					}
					if d.Devcore == 0 {
						t.Error("Devcore should not be zero")
					}
				}
			}
		})
	}
}

func TestGetNodeDevicesReturnsInventoryError(t *testing.T) {
	inventoryPath := writeInventoryFile(t, `
apiVersion: mock.hami.io/v1alpha1
kind: MockInventory
groupBy:
  labelKey: hami.io/mock-group
groups:
  gpu-a100:
    nvidia:
      - id: GPU-MOCK-0
        type: NVIDIA-A100-SXM4-80GB
        devmem: 81920
        devcore: 100
        count: 10
        health: true
`)

	dev := InitNvidiaDevice(NvidiaConfig{}, inventoryPath)
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a100-0",
			Labels: map[string]string{"hami.io/mock-group": "gpu-a100"},
			Annotations: map[string]string{
				RegisterAnnos: `[{"id":"GPU-0","index":0,"count":10,"devmem":40960,"devcore":50,"type":"NVIDIA-Legacy","health":true}]`,
			},
		},
	}

	_, err := dev.GetNodeDevices(&node)
	if err == nil {
		t.Fatalf("expected invalid inventory to return an error")
	}
}

func TestGetNodeDevicesReturnsPairScoreErrorForInventoryNode(t *testing.T) {
	inventoryPath := writeInventoryFile(t, `
apiVersion: mock.hami.io/v1alpha1
kind: MockInventory
groupBy:
  labelKey: hami.io/mock-group
groups:
  gpu-a100:
    nvidia:
      - id: GPU-MOCK-0
        index: 0
        type: NVIDIA-A100-SXM4-80GB
        devmem: 81920
        devcore: 100
        count: 10
        health: true
`)

	dev := InitNvidiaDevice(NvidiaConfig{}, inventoryPath)
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a100-0",
			Labels: map[string]string{"hami.io/mock-group": "gpu-a100"},
			Annotations: map[string]string{
				RegisterGPUPairScore: "not-json",
			},
		},
	}

	_, err := dev.GetNodeDevices(&node)
	if err == nil {
		t.Fatalf("expected malformed pair-score annotation to return an error")
	}
}
