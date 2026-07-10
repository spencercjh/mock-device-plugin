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
	"strings"

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
	Nvidia []*device.DeviceInfo `yaml:"nvidia"`
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

	devs, err := group.nvidiaDevices(groupName, groupData)
	if err != nil {
		return nil, true, err
	}

	return devs, true, nil
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

func (g *Group) nvidiaDevices(groupName string, data any) ([]*device.DeviceInfo, error) {
	rawDevices, err := decodeRawNvidiaDevices(groupName, data)
	if err != nil {
		return nil, err
	}
	if len(rawDevices) != len(g.Nvidia) {
		return nil, fmt.Errorf("groups.%s.nvidia decode mismatch", groupName)
	}

	seenIDs := make(map[string]struct{}, len(g.Nvidia))
	seenIndexes := make(map[uint]struct{}, len(g.Nvidia))
	devs := make([]*device.DeviceInfo, 0, len(g.Nvidia))

	for idx, devInfo := range g.Nvidia {
		if devInfo == nil {
			return nil, fmt.Errorf("groups.%s.nvidia[%d] is nil", groupName, idx)
		}
		if err := validateRequiredFields(groupName, idx, rawDevices[idx], "id", "index", "count", "devmem", "devcore", "type", "health"); err != nil {
			return nil, err
		}
		if strings.TrimSpace(devInfo.ID) == "" {
			return nil, fmt.Errorf("groups.%s.nvidia[%d].id is required", groupName, idx)
		}
		if strings.TrimSpace(devInfo.Type) == "" {
			return nil, fmt.Errorf("groups.%s.nvidia[%d].type is required", groupName, idx)
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

	return cloneDevices(devs), nil
}

func decodeRawNvidiaDevices(groupName string, data any) ([]map[string]any, error) {
	groupYAML, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal raw groups.%s: %w", groupName, err)
	}

	var raw struct {
		Nvidia []map[string]any `yaml:"nvidia"`
	}
	if err := yaml.Unmarshal(groupYAML, &raw); err != nil {
		return nil, fmt.Errorf("decode raw groups.%s: %w", groupName, err)
	}

	return raw.Nvidia, nil
}

func validateRequiredFields(groupName string, index int, raw map[string]any, fields ...string) error {
	if raw == nil {
		return fmt.Errorf("groups.%s.nvidia[%d] is nil", groupName, index)
	}
	for _, field := range fields {
		value, ok := raw[field]
		if !ok || value == nil {
			return fmt.Errorf("groups.%s.nvidia[%d].%s is required", groupName, index, field)
		}
	}
	return nil
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
