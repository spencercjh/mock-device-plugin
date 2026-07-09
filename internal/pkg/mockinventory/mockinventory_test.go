package mockinventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/HAMi/mock-device-plugin/internal/pkg/api/device"
	corev1 "k8s.io/api/core/v1"
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

func TestLoadResolveNvidiaDevicesForMatchingGroup(t *testing.T) {
	path := writeInventoryFile(t, `
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

	inv, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a100-0",
			Labels: map[string]string{"hami.io/mock-group": "gpu-a100"},
		},
	}

	devs, active, err := inv.ResolveNvidiaDevices(node)
	if err != nil {
		t.Fatalf("ResolveNvidiaDevices() error = %v", err)
	}
	if !active {
		t.Fatalf("expected active inventory path")
	}
	if len(devs) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devs))
	}
	if devs[0].ID != "GPU-MOCK-0" {
		t.Fatalf("expected GPU-MOCK-0, got %s", devs[0].ID)
	}
}

func TestResolveNvidiaDevicesWithoutMatchingLabelFallsBack(t *testing.T) {
	path := writeInventoryFile(t, `
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

	inv, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-no-group",
			Labels: map[string]string{},
		},
	}

	devs, active, err := inv.ResolveNvidiaDevices(node)
	if err != nil {
		t.Fatalf("ResolveNvidiaDevices() error = %v", err)
	}
	if active {
		t.Fatalf("expected fallback path, got active inventory path")
	}
	if len(devs) != 0 {
		t.Fatalf("expected 0 devices on fallback, got %d", len(devs))
	}
}

func TestResolveNvidiaDevicesClonesReturnedEntries(t *testing.T) {
	path := writeInventoryFile(t, `
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

	inv, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a100-0",
			Labels: map[string]string{"hami.io/mock-group": "gpu-a100"},
		},
	}

	devs, active, err := inv.ResolveNvidiaDevices(node)
	if err != nil || !active {
		t.Fatalf("ResolveNvidiaDevices() active=%v err=%v", active, err)
	}

	devs[0].Type = "mutated"

	devs2, active, err := inv.ResolveNvidiaDevices(node)
	if err != nil || !active {
		t.Fatalf("ResolveNvidiaDevices() second call active=%v err=%v", active, err)
	}
	if devs2[0].Type != "NVIDIA-A100-SXM4-80GB" {
		t.Fatalf("expected original type to survive clone, got %s", devs2[0].Type)
	}
}

func TestLoadRejectsMissingLabelKey(t *testing.T) {
	path := writeInventoryFile(t, `
apiVersion: mock.hami.io/v1alpha1
kind: MockInventory
groupBy: {}
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

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected missing labelKey error")
	}
}

func TestLoadRejectsDuplicateDeviceIDs(t *testing.T) {
	path := writeInventoryFile(t, `
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
      - id: GPU-MOCK-0
        index: 1
        type: NVIDIA-A100-SXM4-80GB
        devmem: 81920
        devcore: 100
        count: 10
        health: true
`)

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected duplicate id error")
	}
}

func TestLoadRejectsDuplicateIndexes(t *testing.T) {
	path := writeInventoryFile(t, `
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
      - id: GPU-MOCK-1
        index: 0
        type: NVIDIA-A100-SXM4-80GB
        devmem: 81920
        devcore: 100
        count: 10
        health: true
`)

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected duplicate index error")
	}
}

func TestLoadRejectsNegativeNumericFields(t *testing.T) {
	path := writeInventoryFile(t, `
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
        devmem: -1
        devcore: 100
        count: 10
        health: true
`)

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected negative numeric field error")
	}
}

var _ = device.DeviceInfo{}
