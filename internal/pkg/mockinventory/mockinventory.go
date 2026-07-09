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
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	GroupBy    GroupBy          `yaml:"groupBy"`
	Groups     map[string]Group `yaml:"groups"`
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
	if err := yaml.Unmarshal(data, &inv); err != nil {
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

	for groupName, group := range inv.Groups {
		seenIDs := make(map[string]struct{}, len(group.Nvidia))
		seenIndexes := make(map[uint]struct{}, len(group.Nvidia))

		for _, gpu := range group.Nvidia {
			if gpu == nil {
				return fmt.Errorf("groups.%s.nvidia contains nil entry", groupName)
			}
			if _, ok := seenIDs[gpu.ID]; ok {
				return fmt.Errorf("groups.%s.nvidia contains duplicate id %q", groupName, gpu.ID)
			}
			if _, ok := seenIndexes[gpu.Index]; ok {
				return fmt.Errorf("groups.%s.nvidia contains duplicate index %d", groupName, gpu.Index)
			}
			if gpu.Count < 0 || gpu.Devmem < 0 || gpu.Devcore < 0 || gpu.Numa < 0 {
				return fmt.Errorf("groups.%s.nvidia contains negative numeric value", groupName)
			}

			seenIDs[gpu.ID] = struct{}{}
			seenIndexes[gpu.Index] = struct{}{}
		}
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

	group, ok := inv.Groups[groupName]
	if !ok || len(group.Nvidia) == 0 {
		return nil, false, nil
	}

	return cloneDevices(group.Nvidia), true, nil
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
