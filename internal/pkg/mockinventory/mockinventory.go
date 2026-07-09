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

package mockinventory

import (
	"fmt"
	"maps"
	"os"

	"github.com/HAMi/mock-device-plugin/internal/pkg/api/device"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
)

type Inventory struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	GroupBy    GroupBy        `yaml:"groupBy"`
	Groups     map[string]any `yaml:"groups"`
}

type GroupBy struct {
	LabelKey string `yaml:"labelKey"`
}

type Group struct {
	Nvidia []*inventoryDevice `yaml:"nvidia"`
}

type inventoryDevice struct {
	ID              *string                 `yaml:"id"`
	Index           *uint                   `yaml:"index"`
	Count           *int32                  `yaml:"count"`
	Devmem          *int32                  `yaml:"devmem"`
	Devcore         *int32                  `yaml:"devcore"`
	Type            *string                 `yaml:"type"`
	Numa            *int                    `yaml:"numa"`
	Mode            *string                 `yaml:"mode"`
	MIGTemplate     []device.Geometry       `yaml:"migtemplate"`
	Health          *bool                   `yaml:"health"`
	DeviceVendor    *string                 `yaml:"devicevendor"`
	CustomInfo      map[string]any          `yaml:"custominfo"`
	DevicePairScore *device.DevicePairScore `yaml:"devicepairscore"`
}

func Load(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var inv Inventory
	if err := yaml.UnmarshalStrict(data, &inv); err != nil {
		return nil, err
	}
	if err := inv.Validate(); err != nil {
		return nil, err
	}

	return &inv, nil
}

func (inv *Inventory) Validate() error {
	if inv.GroupBy.LabelKey == "" {
		return fmt.Errorf("groupBy.labelKey is required")
	}
	return nil
}

func (inv *Inventory) ResolveNvidiaDevices(node *corev1.Node) ([]*device.DeviceInfo, bool, error) {
	if inv == nil || node == nil {
		return nil, false, nil
	}

	groupName, ok := node.Labels[inv.GroupBy.LabelKey]
	if !ok || groupName == "" {
		return nil, false, nil
	}

	groupData, ok := inv.Groups[groupName]
	if !ok {
		return nil, false, nil
	}

	group, err := decodeGroup(groupName, groupData)
	if err != nil {
		return nil, true, err
	}
	if len(group.Nvidia) == 0 {
		return nil, false, nil
	}

	devs, err := group.nvidiaDevices(groupName)
	if err != nil {
		return nil, true, err
	}

	return cloneDevices(devs), true, nil
}

func decodeGroup(groupName string, data any) (*Group, error) {
	groupYAML, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal groups.%s: %w", groupName, err)
	}

	var group Group
	if err := yaml.UnmarshalStrict(groupYAML, &group); err != nil {
		return nil, fmt.Errorf("decode groups.%s: %w", groupName, err)
	}

	return &group, nil
}

func (g *Group) nvidiaDevices(groupName string) ([]*device.DeviceInfo, error) {
	seenIDs := make(map[string]struct{}, len(g.Nvidia))
	seenIndexes := make(map[uint]struct{}, len(g.Nvidia))
	devs := make([]*device.DeviceInfo, 0, len(g.Nvidia))

	for idx, gpu := range g.Nvidia {
		devInfo, err := gpu.toDeviceInfo(groupName, idx)
		if err != nil {
			return nil, err
		}
		if _, ok := seenIDs[devInfo.ID]; ok {
			return nil, fmt.Errorf("groups.%s.nvidia[%d].id %q is duplicated", groupName, idx, devInfo.ID)
		}
		if _, ok := seenIndexes[devInfo.Index]; ok {
			return nil, fmt.Errorf("groups.%s.nvidia[%d].index %d is duplicated", groupName, idx, devInfo.Index)
		}
		if devInfo.Count < 0 || devInfo.Devmem < 0 || devInfo.Devcore < 0 || devInfo.Numa < 0 {
			return nil, fmt.Errorf("groups.%s.nvidia[%d] contains negative numeric value", groupName, idx)
		}

		seenIDs[devInfo.ID] = struct{}{}
		seenIndexes[devInfo.Index] = struct{}{}
		devs = append(devs, devInfo)
	}

	return devs, nil
}

func (d *inventoryDevice) toDeviceInfo(groupName string, index int) (*device.DeviceInfo, error) {
	if d == nil {
		return nil, fmt.Errorf("groups.%s.nvidia[%d] is nil", groupName, index)
	}

	id, err := requiredString(groupName, index, "id", d.ID)
	if err != nil {
		return nil, err
	}
	deviceType, err := requiredString(groupName, index, "type", d.Type)
	if err != nil {
		return nil, err
	}
	deviceIndex, err := requiredValue(groupName, index, "index", d.Index)
	if err != nil {
		return nil, err
	}
	count, err := requiredValue(groupName, index, "count", d.Count)
	if err != nil {
		return nil, err
	}
	devmem, err := requiredValue(groupName, index, "devmem", d.Devmem)
	if err != nil {
		return nil, err
	}
	devcore, err := requiredValue(groupName, index, "devcore", d.Devcore)
	if err != nil {
		return nil, err
	}
	health, err := requiredValue(groupName, index, "health", d.Health)
	if err != nil {
		return nil, err
	}

	devInfo := &device.DeviceInfo{
		ID:          id,
		Index:       deviceIndex,
		Count:       count,
		Devmem:      devmem,
		Devcore:     devcore,
		Type:        deviceType,
		Health:      health,
		MIGTemplate: cloneMigTemplate(d.MIGTemplate),
		CustomInfo:  maps.Clone(d.CustomInfo),
	}
	if d.Numa != nil {
		devInfo.Numa = *d.Numa
	}
	if d.Mode != nil {
		devInfo.Mode = *d.Mode
	}
	if d.DeviceVendor != nil {
		devInfo.DeviceVendor = *d.DeviceVendor
	}
	if d.DevicePairScore != nil {
		devInfo.DevicePairScore = cloneDevicePairScore(*d.DevicePairScore)
	}

	return devInfo, nil
}

func requiredString(groupName string, index int, field string, value *string) (string, error) {
	result, err := requiredValue(groupName, index, field, value)
	if err != nil {
		return "", err
	}
	if result == "" {
		return "", fmt.Errorf("groups.%s.nvidia[%d].%s is required", groupName, index, field)
	}
	return result, nil
}

func requiredValue[T any](groupName string, index int, field string, value *T) (T, error) {
	var zero T
	if value == nil {
		return zero, fmt.Errorf("groups.%s.nvidia[%d].%s is required", groupName, index, field)
	}
	return *value, nil
}

func cloneDevices(in []*device.DeviceInfo) []*device.DeviceInfo {
	out := make([]*device.DeviceInfo, 0, len(in))
	for _, item := range in {
		if item == nil {
			continue
		}

		cloned := *item
		cloned.CustomInfo = maps.Clone(item.CustomInfo)
		cloned.DevicePairScore = cloneDevicePairScore(item.DevicePairScore)
		cloned.MIGTemplate = cloneMigTemplate(item.MIGTemplate)
		out = append(out, &cloned)
	}

	return out
}

func cloneDevicePairScore(in device.DevicePairScore) device.DevicePairScore {
	in.Scores = maps.Clone(in.Scores)
	return in
}

func cloneMigTemplate(in []device.Geometry) []device.Geometry {
	if in == nil {
		return nil
	}

	out := make([]device.Geometry, 0, len(in))
	for _, geometry := range in {
		if geometry == nil {
			out = append(out, nil)
			continue
		}

		geometryCopy := append(device.Geometry(nil), geometry...)
		out = append(out, geometryCopy)
	}

	return out
}
