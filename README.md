# Mock device plugin for HAMi
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FProject-HAMi%2Fmock-device-plugin.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2FProject-HAMi%2Fmock-device-plugin?ref=badge_shield)

## Introduction

This is a Kubernetes device plugin that registers **virtual** device resources (e.g. `gpu-memory`, `gpu-cores`) -- the resources HAMi tracks but the kubelet would normally ignore -- onto a node, **without requiring real hardware**. It lets you test the HAMi scheduler (scheduling policies, quotas, webhooks) on machines that have no GPU/NPU.

After deployment these resources show up under `node.status.allocatable` and `node.status.capacity`.

## How it works (read this first)

The mock plugin **does not detect hardware**. It refreshes every ~30s and has two input paths:

```text
  recommended for NVIDIA:
    ConfigMap file /mock-inventory/mock-inventory.yaml
      -> groupBy.labelKey selects the node label
      -> groups.<label-value>.nvidia[] becomes the active device list

  legacy fallback:
    node annotation: hami.io/node-<vendor>-register = [ {devmem, devcore, ...}, ... ]   (1)
    node capacity:   <count resource> (e.g. nvidia.com/gpu) > 0                          (2)  <- health gate

  active device list
      -> registers into allocatable: <vendor>/...mem , <vendor>/...cores , ...
```

Today that means:

- **NVIDIA primary path:** mount the optional `hami-mock-inventory` ConfigMap, label the node with `groupBy.labelKey`, and let the plugin read `groups.<group>.nvidia[]`.
- **NVIDIA fallback path:** if the inventory file is missing, the node label is absent, or the label does not match a populated group, the plugin falls back to the legacy manual path.
- **NVIDIA inventory errors:** if the current node matches a group and that group's NVIDIA inventory is malformed or unreadable, the runtime surfaces the error instead of silently falling back.
- **Ascend / Hygon:** these vendors still use the legacy manual path today.

On a **real** cluster, the legacy annotation and count resource are normally produced by the real device plugin. In a **mock-only** (no hardware) environment you provide them yourself when using the fallback/manual path:

- **(1) the `node-<vendor>-register` annotation** describing the fake cards -- `kubectl annotate`.
- **(2) the count extended resource** (e.g. `nvidia.com/gpu`) -- patched onto the node `status`.

> Inventory updates are **not instant**. ConfigMap-to-file propagation is kubelet-driven (commonly up to about 1 minute), and the mock plugin only refreshes every 30s, so end-to-end visibility can lag by up to roughly **90 seconds** after you change the ConfigMap or node label.

## Prerequisites

- Kubernetes >= v1.18
- The `hami-scheduler-device` ConfigMap (the device config). If HAMi is installed it already exists; otherwise create it from [device-configmap.yaml](https://github.com/Project-HAMi/HAMi/blob/master/charts/hami/templates/scheduler/device-configmap.yaml).
- Optional but recommended for NVIDIA: a `hami-mock-inventory` ConfigMap containing `mock-inventory.yaml`. The shipped DaemonSet mounts it as an optional directory, so the plugin can still start and fall back to the legacy path when the ConfigMap is absent.

## Deployment

```bash
make deploy        # = kubectl apply -f k8s-mock-rbac.yaml && kubectl apply -f k8s-mock-plugin.yaml
# or manually:
kubectl apply -f k8s-mock-rbac.yaml
kubectl apply -f k8s-mock-plugin.yaml
```

## Understanding the values

The mock **derives the registered resources from the active device list, not from the count resource.** This trips people up, so to be explicit:

- For **NVIDIA on the recommended path**, the active device list is `groups.<matched-node-group>.nvidia[]` from `mock-inventory.yaml`.
- For **legacy/manual mode** (including Ascend and Hygon), the active device list is the `node-<vendor>-register` annotation.

- **Number of fake cards = the number of entries in the active device list** (not the count value).
- Registered `...-memory` = **sum of `devmem`** over all entries.
- Registered `...-cores` / `...-core` = **sum of `devcore`** over all entries.
- On the **legacy/manual path**, the count extended resource is only a health gate: its value just needs to be `> 0`. It does **not** affect the registered memory/cores. By convention it is set to `cards x splits-per-card` (e.g. Ascend `2 x VDeviceCount(4) = 8`), but `1` would work equally well for the memory/cores to appear.
- On the **NVIDIA inventory-active path**, the plugin returns file-backed resources directly and bypasses that legacy health-gate check. Keep the `nvidia.com/gpu` patch in your mock setup because operators typically still want the vendor count resource present on the node, but it is not what drives the inventory-backed `gpumem` / `gpucores` totals.

Device entry fields:

| field | meaning |
| :-- | :-- |
| `id` | unique device UUID (any string) |
| `devmem` | per-card memory in MB -- **summed** into `...-memory` |
| `devcore` | per-card cores. **NVIDIA/Hygon:** summed into `...-cores` (NVIDIA: percentage, 100 = a whole card). **Ascend:** ignored -- `huawei.com/<chip>-core` is percentage-based, registered as **100 per card**. |
| `count` | per-card split count (informational for the mock) |
| `type` | device model string |
| `health` | must be `true` to be counted |
| `index` | card index `0,1,2,...`; required for inventory-backed entries, optional on the legacy/manual annotation path (defaults to `0` if omitted) |
| `numa`, `mode` | optional |

> **Worked example (Ascend, below):** the annotation has **2 entries**, each `devmem=32768`. So `...-memory = 2x32768 = 65536` and `...-core = 2x100 = 200` (Ascend `...-core` is **percentage-based**: a whole card is always **100**). The count resource `=8` is a separate health-gate value and is unrelated to these numbers.

## Usage by vendor

To mock one card you always need the vendor **config block** (in the `hami-scheduler-device` ConfigMap, gives the resource names) plus an **active device list** that describes the fake cards. For NVIDIA the active list can come from the recommended ConfigMap inventory or the legacy node annotation; for Ascend and Hygon it still comes from the node annotation. In most end-to-end mock setups you will also patch the vendor **count extended resource** onto the node. On the legacy/manual path that count resource is the health gate; on the NVIDIA inventory-active path it is still commonly present on the node, but it is not what drives the inventory-backed resource totals. Replace `<node>` below with your target node. Resource names follow the ConfigMap (HAMi defaults shown). See [Understanding the values](#understanding-the-values) for how the numbers are derived.

### NVIDIA (e.g. A100-80GB)

- config block: `nvidia:` | annotation: `hami.io/node-nvidia-register` (JSON) | count: `nvidia.com/gpu`
- mock registers: `nvidia.com/gpumem`, `nvidia.com/gpucores`, `nvidia.com/gpumem-percentage`

#### Recommended: ConfigMap + node-group label

Create a file-backed inventory, label the node into one of its groups, and keep the `nvidia.com/gpu` patch in place as part of the overall mock node setup:

```bash
cat > mock-inventory.yaml <<'EOF'
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
EOF

kubectl -n kube-system create configmap hami-mock-inventory \
  --from-file=mock-inventory.yaml=./mock-inventory.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl label node <node> hami.io/mock-group=gpu-a100 --overwrite

kubectl patch node <node> --subresource=status --type=json \
  -p '[{"op":"add","path":"/status/capacity/nvidia.com~1gpu","value":"10"}]'

# verify after ConfigMap sync + plugin refresh (allow up to ~90s)
kubectl get node <node> -o json | jq '.status.allocatable|with_entries(select(.key|test("nvidia.com")))'
# expect: nvidia.com/gpu=10, nvidia.com/gpumem=81920, nvidia.com/gpucores=100, nvidia.com/gpumem-percentage=100
```

If the ConfigMap/file is missing, the node label is absent, or the label does not match a populated group, the plugin falls back to the legacy annotation path below.

If the current node matches a group whose NVIDIA inventory is malformed or otherwise unreadable, that is not a silent fallback case. Fix that matched group or remove the inventory input so the plugin can resume normal operation.

#### Legacy fallback: manual annotation

```bash
# (2) count resource: splitCount=10 -> one card advertises 10
kubectl patch node <node> --subresource=status --type=json \
  -p '[{"op":"add","path":"/status/capacity/nvidia.com~1gpu","value":"10"}]'
# (1) annotation: 1 x A100-80G (devmem=81920 MB, devcore=100)
kubectl annotate node <node> \
  'hami.io/node-nvidia-register=[{"id":"GPU-MOCK-0","count":10,"devmem":81920,"devcore":100,"type":"NVIDIA-A100-SXM4-80GB","health":true,"numa":0,"mode":"hami-core"}]'
# verify (~30s later)
kubectl get node <node> -o json | jq '.status.allocatable|with_entries(select(.key|test("nvidia.com")))'
# expect: nvidia.com/gpu=10, nvidia.com/gpumem=81920, nvidia.com/gpucores=100, nvidia.com/gpumem-percentage=100
```

### Ascend NPU (e.g. 910B4)

- config block: `vnpus.configs` (entry for `910B4`) | annotation: `hami.io/node-register-Ascend910B4` (JSON) | count: `huawei.com/Ascend910B4`
- mock registers: `huawei.com/Ascend910B4-memory`, and -- **only when the node runs in `hami-vnpu-core` soft mode** (see below) -- `huawei.com/Ascend910B4-core`

```bash
# (2) count resource: 2 cards x VDeviceCount(4) = 8
kubectl patch node <node> --subresource=status --type=json \
  -p '[{"op":"add","path":"/status/capacity/huawei.com~1Ascend910B4","value":"8"}]'
# (1) annotation: 2 x 910B4 (devmem=32768 MB, devcore=20), matching the real ascend-device-plugin report
kubectl annotate node <node> \
  'hami.io/node-register-Ascend910B4=[{"id":"MOCK-0","count":4,"devmem":32768,"devcore":20,"type":"Ascend910B4","health":true},{"id":"MOCK-1","index":1,"count":4,"devmem":32768,"devcore":20,"type":"Ascend910B4","health":true}]'
# (1b) soft mode: mark the node as hami-vnpu-core so the -core resource is reported.
# Either set vnpus.hamiVnpuCore: true globally in the ConfigMap, or annotate this node
# (the node annotation overrides the global setting, same precedence as the real plugin):
kubectl annotate node <node> hami-vnpu-core=true
# verify
kubectl get node <node> -o json | jq '.status.allocatable|with_entries(select(.key|test("Ascend910B4")))'
# expect: huawei.com/Ascend910B4-memory=65536, huawei.com/Ascend910B4-core=200  (2 cards x 100)
```

The Ascend `-core` resource is **percentage-based**: each physical card contributes **100** (a whole card), independent of the annotation's `devcore`. HAMi caps a core request at 100 and, in `hami-vnpu-core` soft mode, treats a card's total core as 100.

**When is `-core` reported?** Only on a node that runs in `hami-vnpu-core` (soft-partition) mode, mirroring the real ascend-device-plugin and the HAMi scheduler. Mode is decided by: the node's `hami-vnpu-core` annotation if present, otherwise the global `vnpus.hamiVnpuCore` in the ConfigMap (`true`/`false`, default `false`). On a hard/template node (`hami-vnpu-core` off) the HAMi scheduler filters out any pod that requests `-core` (`ModeNotFit`), so the mock does not register `-core` there. There is **no ascend-device-plugin** in a mock environment, so nothing writes the `hami-vnpu-core` node annotation automatically -- set it yourself (like the `node-register` annotation and the count resource).

### Hygon DCU

- config block: `hygon:` | annotation: `hami.io/node-dcu-register` (**CSV**, not JSON) | count: `hygon.com/dcunum`
- mock registers: `hygon.com/dcumem`, and -- when the config sets `resourceCoreName` -- `hygon.com/dcucores` (sum of per-card `devcore`)

```bash
# (2) count resource: 2 DCUs
kubectl patch node <node> --subresource=status --type=json \
  -p '[{"op":"add","path":"/status/capacity/hygon.com~1dcunum","value":"2"}]'
# (1) annotation: CSV form is "id,count,devmem,devcore,type,numa,health,index,mode:" per card
kubectl annotate node <node> \
  'hami.io/node-dcu-register=DCU-MOCK-0,2,16384,100,Z100L,0,true,0,hami-core:DCU-MOCK-1,2,16384,100,Z100L,1,true,1,hami-core:'
# verify
kubectl get node <node> -o json | jq '.status.allocatable|with_entries(select(.key|test("hygon.com")))'
# expect: hygon.com/dcumem=32768, hygon.com/dcucores=200  (2 cards x devcore 100)
```

## Ascend config compatibility (new vs legacy)

The Ascend `vnpus` config has two layouts and the plugin accepts **both**:

```yaml
# New (HAMi >= v2.9.0): nested object
vnpus:
  hamiVnpuCore: true          # node runs in soft mode -> the -core resource is reported
  configs:
    - chipName: 910B4
      resourceCoreName: huawei.com/Ascend910B4-core   # name of the -core resource
      ...
```
```yaml
# Legacy (older / downstream): a bare list
vnpus:
  - chipName: 910B4
    ...
```
The new nested format is tried first; if that fails it falls back to the legacy flat list; if neither matches an error is returned. This lets clusters that have not upgraded their ConfigMap keep working.

## ManagedResources

| Devices    | Mocking Resources |
| :---       | :----   |
| Nvidia GPU | `nvidia.com/gpumem`, `nvidia.com/gpumem-percentage`, `nvidia.com/gpucores` |
| Hygon DCU  | `hygon.com/dcumem`, `hygon.com/dcucores` (when `resourceCoreName` is set) |
| Ascend     | `huawei.com/Ascend{chip}-memory`, `huawei.com/Ascend{chip}-core` (when `resourceCoreName` is set **and** the node is in `hami-vnpu-core` mode) |

**Note:** If the counted memory is too large (e.g. > 120GB) it may display as 0. Set `memoryFactor` in the `hami-scheduler-device` ConfigMap (default 1).

## Build

```bash
make build         # build the binary into bin/
make test          # run unit tests
make docker-build  # build the image (override: IMG=myrepo/mock:tag)
make help          # list all targets
```

## Maintainer

limengxuan@4paradigm.com

## License
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FProject-HAMi%2Fmock-device-plugin.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2FProject-HAMi%2Fmock-device-plugin?ref=badge_large)
