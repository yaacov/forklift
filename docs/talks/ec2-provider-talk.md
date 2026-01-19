# EC2 Provider: Migrating AWS EC2 Instances to OpenShift Virtualization

**Duration:** ~30 minutes  
**Target Audience:** Platform Engineers, DevOps, Cloud Architects

---

## Agenda

1. **About AWS EC2 Virtualization** (~10 min)
   - Instance types: Xen, Nitro, and Metal
   - Regions and Availability Zones
   - Virtualized hardware: NVMe and Nitro Network Card
   - Storage: EBS and S3
   - Networking fundamentals

2. **EC2 to ROSA/OCP Migration** (~10 min)
   - Migration flow overview
   - Limitations (same region requirement)
   - Possibilities and use cases

3. **Live Demo** (~10 min)
   - Creating providers and exploring inventory
   - Creating migration plans (with/without conversion)
   - Debugging with EC2 tags and kubectl tools

---

## Part 1: AWS EC2 Virtualization Fundamentals

### 1.1 EC2 Instance Types: The Evolution

#### Three Generations of Virtualization

```
+-----------------------------------------------------------------------------+
|                     EC2 Virtualization Evolution                            |
+-----------------------------------------------------------------------------+
|                                                                             |
|   Xen (2006)            Nitro (2017)            Metal (2018+)               |
|   +-- PV & HVM modes    +-- Custom hypervisor   +-- No hypervisor           |
|   +-- Type 1            +-- Hardware offload    +-- Bare metal access       |
|   +-- Software-based    +-- NVMe/ENA drivers    +-- Nested virtualization   |
|   +-- Limited perf      +-- Near-bare-metal     +-- Full hardware control   |
|                                                                             |
+-----------------------------------------------------------------------------+
```

| Generation | Technology | Characteristics | Instance Examples |
|------------|-----------|-----------------|-------------------|
| **Xen** | PV + HVM modes | Software hypervisor, older generation | t2.*, m3.*, c3.* |
| **Nitro** | Custom silicon | Hardware-offloaded hypervisor | t3.*, m5.*, c5.*, r5.* |
| **Metal** | No hypervisor | Direct hardware access | m5.metal, c5.metal, i3.metal |

#### What is HVM (Hardware Virtual Machine) Mode?

HVM (Hardware Virtual Machine) is a virtualization mode that uses **hardware-assisted virtualization** extensions (Intel VT-x, AMD-V) to run **unmodified guest operating systems**:

| Mode | Description | Guest OS Modifications |
|------|-------------|----------------------|
| **PV (Paravirtual)** | Guest OS uses special Xen drivers (xen-blkfront, xen-netfront) | Required - PV-aware kernel |
| **HVM** | Hardware virtualization, emulated devices | None - any OS works, but slower I/O |
| **PV-on-HVM** | HVM mode with PV drivers for better I/O performance | Optional - PV drivers recommended |

**Key Points:**
- Early EC2 used **PV mode** requiring modified guest kernels
- Modern Xen instances use **HVM** with optional PV drivers for I/O
- **Nitro** instances are exclusively HVM-based with custom hardware
- For migration: Both PV and HVM drivers need conversion to **VirtIO**

#### What is Instance Store?

**Instance Store** (also called "ephemeral storage") is temporary block storage **physically attached** to the host machine:

```
+-----------------------------------------------------------------------------+
|                    Instance Store vs EBS                                    |
+-----------------------------------------------------------------------------+
|                                                                             |
|   Instance Store (Ephemeral)           EBS (Persistent)                     |
|   +---------------------------+        +---------------------------+        |
|   | - Physically on host      |        | - Network-attached        |        |
|   | - Lost on stop/terminate  |        | - Persists independently  |        |
|   | - Very high IOPS          |        | - Snapshottable           |        |
|   | - Free with instance      |        | - Charged separately      |        |
|   | - /dev/nvme1n1 (Nitro)    |        | - /dev/nvme0n1 (Nitro)    |        |
|   +---------------------------+        +---------------------------+        |
|                                                                             |
|   NOT migrated!                        Fully supported!                     |
|   (Data lost when instance stops)      (Snapshotted and transferred)        |
|                                                                             |
+-----------------------------------------------------------------------------+
```

**Why Instance Store Cannot Be Migrated:**
1. **No snapshot API** - AWS doesn't support snapshotting instance store volumes
2. **Ephemeral by design** - Data is lost when instance stops (which we require)
3. **Host-local** - Physically tied to the specific host hardware

**Workaround:** Before migration, copy important data from instance store to EBS volumes.

#### Why This Matters for Migration

- **Xen instances (PV/HVM)** use `xen-blkfront` / `xen-netfront` drivers → Need conversion to VirtIO
- **Nitro instances** use NVMe/ENA drivers → Need conversion to VirtIO
- **Metal instances** support nested virtualization → Can run KubeVirt directly!

### 1.2 Regions and Availability Zones

```
+-----------------------------------------------------------------------------+
|                    AWS Global Infrastructure                                |
+-----------------------------------------------------------------------------+
|                                                                             |
|   Region: us-east-1 (N. Virginia)                                           |
|   +-- AZ: us-east-1a  <--- OpenShift Worker Nodes                           |
|   |   +-- EBS Volumes MUST be in same AZ as nodes!                          |
|   +-- AZ: us-east-1b                                                        |
|   +-- AZ: us-east-1c                                                        |
|   +-- AZ: us-east-1d                                                        |
|                                                                             |
|   CRITICAL: EBS volumes are AZ-specific!                                    |
|   Snapshots can cross AZs, but final volumes must match node AZ.            |
|                                                                             |
+-----------------------------------------------------------------------------+
```

**Key Points:**
- EC2 instances live in a specific AZ
- EBS volumes are AZ-locked (can't attach to instances in other AZs)
- **Snapshots are region-wide** (this is how we handle AZ transitions!)
- Cross-region migration requires snapshot copy (not currently supported)

#### How Cross-AZ Snapshots Work

This is the key mechanism that enables EC2 migration:

```
+------------------------------------------------------------------------------+
|                    Cross-AZ Migration via Snapshots                          |
+------------------------------------------------------------------------------+
|                                                                              |
|   Source AZ (us-east-1a)            Target AZ (us-east-1c)                   |
|   +---------------------+           +---------------------+                  |
|   | EC2 Instance        |           | OpenShift Nodes     |                  |
|   | +-----------------+ |           | +-----------------+ |                  |
|   | | EBS Volume      | |           | | Worker Node     | |                  |
|   | | vol-source      | |           | | (needs vol here)| |                  |
|   | +--------+--------+ |           | +--------^--------+ |                  |
|   +----------|----------+           +----------|----------+                  |
|              |                                 |                             |
|              | CreateSnapshot                  |                             |
|              v                                 |                             |
|   +--------------------------------------------|--------------------------+  |
|   |              AWS Region (us-east-1)        |                          |  |
|   |         +---------------------+            |                          |  |
|   |         |   EBS Snapshot      |            |                          |  |
|   |         |   snap-xxx          |------------+                          |  |
|   |         |   (region-wide,     |  CreateVolume(AZ=us-east-1c)          |  |
|   |         |    not AZ-specific!)|                                       |  |
|   |         +---------------------+                                       |  |
|   |                                      +---------------------+          |  |
|   |                                      | NEW EBS Volume      |          |  |
|   |                                      | vol-target          |          |  |
|   |                                      | (in us-east-1c!)    |          |  |
|   |                                      +---------------------+          |  |
|   +-----------------------------------------------------------------------+  |
|                                                                              |
|   Snapshots are REGION-WIDE (not AZ-specific like volumes)                   |
|   CreateVolume API accepts AvailabilityZone parameter                        |
|   This enables creating volumes in ANY AZ from the same snapshot             |
|                                                                              |
+------------------------------------------------------------------------------+
``` 

**The Code:**

```go
// pkg/provider/ec2/controller/client/snapshot.go

// 1. Get target AZ from provider settings
targetAZ, _ := r.getTargetClusterAZ()  // e.g., "us-east-1c"

// 2. Create volume in TARGET AZ from the snapshot
createVolInput := &ec2.CreateVolumeInput{
    SnapshotId:       aws.String(snapshotID),      // Snapshot (region-wide)
    AvailabilityZone: aws.String(targetAZ),        // TARGET AZ - the magic!
    VolumeType:       originalVolume.VolumeType,   // Preserve type
}

volume, _ := targetClient.CreateVolume(ctx, createVolInput)
```

**Key Insight:** The `CreateVolume` API lets you specify **any AZ in the region** when creating from a snapshot. This is how AWS enables cross-AZ data movement without copying bytes yourself.

### 1.3 Virtualized Hardware: NVMe and Nitro Network Card

#### NVMe Storage (Nitro Instances)

```
EC2 Instance (Nitro)
├─ /dev/nvme0n1  ─── Root EBS Volume (gp3)
├─ /dev/nvme1n1  ─── Data EBS Volume (io2)
└─ /dev/nvme2n1  ─── Instance Store (ephemeral) - NOT MIGRATED
```

| Device Type | Migration Support | Notes |
|-------------|------------------|-------|
| EBS Volumes | Full support | Snapshotted and migrated |
| Instance Store | Not supported | Ephemeral, data lost on stop |

#### Elastic Network Adapter (ENA)

- High-performance networking (up to 100 Gbps on some instances)
- Replaced the old Intel 82599 VF (ixgbevf) driver
- **Migration impact:** Converted to VirtIO or E1000e

### 1.4 EC2 Storage: EBS and S3

AWS provides two primary storage services:

| Service | Type | Use Case | AZ Scope |
|---------|------|----------|----------|
| **EBS** | Block storage | VM disks, databases | AZ-specific |
| **S3** | Object storage | Files, backups, archives | Regional |

**Where are EBS Snapshots stored?**
- EBS snapshots are stored in **AWS-owned S3** (not your S3 buckets)
- This is why snapshots are **region-wide** while EBS volumes are AZ-specific
- You don't see snapshots in your S3 console - they're managed by AWS internally

#### Elastic Block Store (EBS)

#### Elastic Block Store (EBS)

```
+-----------------------------------------------------------------------------+
|                         EBS Volume Types                                    |
+-----------------------------------------------------------------------------+
|                                                                             |
|   General Purpose SSD        Provisioned IOPS SSD       Throughput HDD      |
|   +------------------+       +-------------------+      +--------------+    |
|   | gp2   | gp3      |       | io1   | io2       |      | st1   | sc1  |    |
|   | 3K    | 16K IOPS |       | 64K   | 256K IOPS |      | 500   | 250  |    |
|   | IOPS  | (max)    |       | IOPS  | (max)     |      | MiBps | MiBps|    |
|   +-------+----------+       +-------+-----------+      +-------+------+    |
|                                                                             |
|   All EBS types are supported for migration!                                |
|                                                                             |
+-----------------------------------------------------------------------------+
```

| Volume Type | Use Case | Max Size | Migrated To |
|-------------|----------|----------|-------------|
| gp2/gp3 | General workloads | 16 TiB | StorageClass of your choice |
| io1/io2 | Databases, high IOPS | 64 TiB | StorageClass of your choice |
| st1 | Big data, streaming | 16 TiB | StorageClass of your choice |
| sc1 | Cold storage, archives | 16 TiB | StorageClass of your choice |
| standard | Magnetic (legacy) | 1 TiB | StorageClass of your choice |

#### Storage Mapping and the EBS CSI Driver Constraint

**Important:** The EC2 provider creates PVs that are **hardcoded to use the EBS CSI driver**:

```go
// From pkg/provider/ec2/controller/builder/volumes.go
const EBSCSIDriver = "ebs.csi.aws.com"

pv := &core.PersistentVolume{
    Spec: core.PersistentVolumeSpec{
        PersistentVolumeSource: core.PersistentVolumeSource{
            CSI: &core.CSIPersistentVolumeSource{
                Driver:       EBSCSIDriver,              // HARDCODED!
                VolumeHandle: volumeInfo.EBSVolumeID,    // Direct EBS volume reference
            },
        },
    },
}
```

**What does this mean?**

1. **The migrated volumes ARE EBS volumes** - they stay as EBS in AWS
2. **The StorageClass MUST use the EBS CSI provisioner** (`ebs.csi.aws.com`)
3. **You cannot migrate to ODF, local-path, or other storage backends**

```yaml
# The StorageMap mapping is used for labeling/organization:
spec:
  map:
    - source:
        name: gp3           # Source EBS volume type
      destination:
        storageClass: gp3-csi    # MUST be EBS CSI-backed StorageClass
```

**Why this design?**
- **Zero data copy** - We don't transfer bytes, we create new EBS volumes from snapshots
- **Maximum performance** - Native EBS attachment via CSI
- **Simplicity** - No intermediate storage or data movement

**The tradeoff:** If you need the data on non-EBS storage (ODF, local, etc.), you'd need a post-migration copy step.

#### Zone Constraint and Node Scheduling

EBS volumes can **only attach to nodes in the same Availability Zone**. The EC2 provider handles this automatically:

```
+-----------------------------------------------------------------------------+
|                    AZ-Aware Scheduling                                      |
+-----------------------------------------------------------------------------+
|                                                                             |
|   Provider Config                       Automatic Node Selector             |
|   +---------------------+              +---------------------------------+  |
|   | spec:               |              | spec:                           |  |
|   |   settings:         | -----------> |   template:                     |  |
|   |     target-az:      |              |     spec:                       |  |
|   |       us-east-1a    |              |       nodeSelector:             |  |
|   +---------------------+              |         topology.kubernetes.io/ |  |
|                                        |           zone: us-east-1a      |  |
|                                        +---------------------------------+  |
|                                                                             |
|   Applied to:                                                               |
|   - Migrated VirtualMachine                                                 |
|   - virt-v2v Conversion Pod                                                 |
|                                                                             |
+-----------------------------------------------------------------------------+
```

**The `target-az` must match where your OpenShift worker nodes are running!**

```bash
# Check which AZs have worker nodes:
kubectl get nodes -L topology.kubernetes.io/zone

# Example output:
NAME                           STATUS   ZONE
ip-10-0-1-100.ec2.internal     Ready    us-east-1a  ◄── Use this AZ
ip-10-0-2-200.ec2.internal     Ready    us-east-1a
ip-10-0-3-300.ec2.internal     Ready    us-east-1b
```

**If `target-az` doesn't match your worker nodes:**
- EBS volumes will be created in the wrong AZ
- CSI driver cannot attach volumes
- Migration will fail at PVC binding stage

#### Migration Data Flow

```
+-----------------------------------------------------------------------------+
|                         EBS Migration Data Flow                             |
+-----------------------------------------------------------------------------+
|                                                                             |
|   Source (any AZ)                                        Target AZ          |
|   +--------------+                                      +--------------+    |
|   | EC2          |                                      | OpenShift    |    |
|   | Instance     |                                      | KubeVirt     |    |
|   +------+-------+                                      +------^-------+    |
|          |                                                     |            |
|   +------v-------+   CreateSnapshot    +---------------+       |            |
|   | EBS Volume   | ------------------> | EBS Snapshot  |       |            |
|   | vol-xxx      |                     | snap-xxx      |       |            |
|   | (AZ: 1a)     |                     | (regional)    |       |            |
|   +--------------+                     +-------+-------+       |            |
|                                                |               |            |
|                          CreateVolume(AZ=1c)   |               |            |
|                                                v               |            |
|                                        +---------------+       |            |
|                                        | NEW EBS Vol   |  CSI  |            |
|                                        | vol-yyy       |-------+            |
|                                        | (AZ: 1c)      |                    |
|                                        +---------------+                    |
|                                                                             |
+-----------------------------------------------------------------------------+
```

**Key:** Snapshots are **region-wide**, so `CreateVolume` can specify any AZ in the region.

### 1.5 EC2 Networking

#### VPC and Subnet Basics

| Concept | Description | Scope |
|---------|-------------|-------|
| **VPC** | Virtual Private Cloud - isolated network environment | Regional |
| **Subnet** | Subdivision of VPC with specific CIDR block | AZ-specific |
| **CIDR** | IP address range notation (e.g., `10.0.0.0/16`) | - |
| **ENI** | Elastic Network Interface - virtual NIC attached to instance | AZ-specific |
| **Security Group** | Firewall rules (stateful) | VPC-wide |

#### What is CIDR?

**CIDR** (Classless Inter-Domain Routing) is a notation for specifying IP address ranges:

```
   10.0.0.0/16
   ────┬────  ─┬─
       │       └── Prefix length: how many bits are the network part
       └────────── Network address

   /16 = 16 bits for network, 16 bits for hosts = 65,536 addresses
   /24 = 24 bits for network, 8 bits for hosts  = 256 addresses
```

| CIDR | Addresses | Typical Use |
|------|-----------|-------------|
| `/16` | 65,536 | VPC (e.g., `10.0.0.0/16`) |
| `/24` | 256 | Subnet (e.g., `10.0.1.0/24`) |
| `/32` | 1 | Single host |

```
+-----------------------------------------------------------------------------+
|                    EC2 Network Architecture                                 |
+-----------------------------------------------------------------------------+
|                                                                             |
|   VPC: 10.0.0.0/16 (Regional - spans all AZs)                               |
|   |                                                                         |
|   +-- Subnet: 10.0.1.0/24 (Public, us-east-1a)                              |
|   |   +-- ENI: eni-abc123 --> EC2 Instance                                  |
|   |       +-- Private IP: 10.0.1.50                                         |
|   |       +-- Public IP: 54.x.x.x (Elastic IP)                              |
|   |       +-- MAC: 02:xx:xx:xx:xx:xx <-- Preserved in migration!            |
|   |                                                                         |
|   +-- Subnet: 10.0.2.0/24 (Private, us-east-1a)                             |
|   |                                                                         |
|   +-- Subnet: 10.0.3.0/24 (Private, us-east-1c)  <-- Different AZ           |
|                                                                             |
|   Key: VPC spans region, Subnets are AZ-specific                            |
|                                                                             |
+-----------------------------------------------------------------------------+
```

#### Network Migration Mapping

The EC2 provider maps **subnets** (not VPCs) to target networks:

```
+-----------------------------------------------------------------------------+
|                    Network Mapping Options                                  |
+-----------------------------------------------------------------------------+
|                                                                             |
|   EC2 Subnet                       OpenShift Target                         |
|   +-----------------+             +-------------------------------------+   |
|   | subnet-abc123   |             |                                     |   |
|   | 10.0.1.0/24     | ----------> |  Option 1: Pod Network (default)    |   |
|   +-----------------+             |  - Simple, uses cluster SDN         |   |
|                                   |  - Masquerade NAT for egress        |   |
|                                   |  - With UDN: uses l2bridge binding  |   |
|   +-----------------+             |                                     |   |
|   | subnet-def456   |             |  Option 2: Multus                   |   |
|   | 10.0.2.0/24     | ----------> |  - Bridge to external network       |   |
|   +-----------------+             |  - Direct L2 connectivity           |   |
|                                   |                                     |   |
|   +-----------------+             |  Option 3: Ignored                  |   |
|   | subnet-ghi789   |             |  - Skip this network interface      |   |
|   | 10.0.3.0/24     | ----------> |  - Interface not created in target  |   |
|   +-----------------+             |                                     |   |
|                                   +-------------------------------------+   |
|                                                                             |
+-----------------------------------------------------------------------------+
```

#### NetworkMap Example

```yaml
apiVersion: forklift.konveyor.io/v1beta1
kind: NetworkMap
metadata:
  name: ec2-network-map
spec:
  map:
    - source:
        id: subnet-abc123      # EC2 Subnet ID
      destination:
        type: pod              # Pod network (uses UDN l2bridge if enabled)
    - source:
        id: subnet-def456
      destination:
        type: multus
        namespace: default
        name: my-bridge-net    # NetworkAttachmentDefinition name
    - source:
        id: subnet-ghi789
      destination:
        type: ignored          # Skip this interface
```

| Network Type | Use Case | MAC Preservation |
|--------------|----------|------------------|
| **pod** | Simple workloads, cluster networking | Yes (with UDN enabled) |
| **multus** | L2 connectivity to external networks | Yes |
| **ignored** | Skip interface (not needed in target) | N/A |

#### UDN (User Defined Networks)

UDN is enabled at the **cluster level** (OCP 4.15+). When UDN is enabled:
- Pod networks automatically use `l2bridge` binding
- Better MAC address preservation support
- OVN-based network isolation

**Note:** There's no separate "udn" type in the NetworkMap - you use `type: pod` and the cluster's UDN configuration determines the behavior.

#### What is L2Bridge Binding?

In KubeVirt, **binding** defines how a VM's virtual NIC connects to the network:

```
+-----------------------------------------------------------------------------+
|                    KubeVirt Network Bindings                                |
+-----------------------------------------------------------------------------+
|                                                                             |
|   Masquerade (default)                L2Bridge (with UDN)                   |
|   +---------------------+             +---------------------+               |
|   | VM                  |             | VM                  |               |
|   | +-----------------+ |             | +-----------------+ |               |
|   | | eth0            | |             | | eth0            | |               |
|   | | 10.0.2.2        | |             | | 10.128.0.50     | |               |
|   | | (private NAT)   | |             | | (pod network IP)| |               |
|   | +--------+--------+ |             | +--------+--------+ |               |
|   +----------|----------+             +----------|----------+               |
|              |                                   |                          |
|              v                                   v                          |
|   +---------------------+             +---------------------+               |
|   | NAT (iptables)      |             | Bridge (L2)         |               |
|   | VM IP -> Pod IP     |             | Direct connection   |               |
|   +---------------------+             +---------------------+               |
|                                                                             |
|   MAC not preserved                   MAC preserved                         |
|   No inbound connections              L2 connectivity                       |
|   Simple, works everywhere            Requires UDN/bridge support           |
|                                                                             |
+-----------------------------------------------------------------------------+
```

| Binding | MAC Preserved | Inbound Traffic | Use Case |
|---------|---------------|-----------------|----------|
| **masquerade** | No | Via Services only | Default, simple |
| **l2bridge** | Yes | Direct L2 | UDN, MAC-dependent apps |
| **bridge** | Yes | Direct L2 | Multus networks |

---

## Part 2: EC2 to ROSA/OCP Migration

### 2.1 Migration Flow Overview

```
+-----------------------------------------------------------------------------+
|                    EC2 Migration Pipeline                                   |
+-----------------------------------------------------------------------------+
|                                                                             |
|   1. INITIALIZE                                                             |
|      +-- Validate VM, initialize tracking                                   |
|                                                                             |
|   2. PREPARE SOURCE                                                         |
|      +-- Stop EC2 instance (ensure data consistency)                        |
|                                                                             |
|   3. CREATE SNAPSHOTS                                                       |
|      +-- Create EBS snapshots for all attached volumes                      |
|      +-- Tag snapshots: forklift.konveyor.io/vmID=i-xxxx                    |
|                                                                             |
|   4. SHARE SNAPSHOTS (cross-account only)                                   |
|      +-- Share snapshots with target AWS account                            |
|                                                                             |
|   5. DISK TRANSFER                                                          |
|      +-- Create new EBS volumes from snapshots in target AZ                 |
|      +-- Create PersistentVolumes (CSI volumeHandle)                        |
|      +-- Create PersistentVolumeClaims (pre-bound)                          |
|                                                                             |
|   6. IMAGE CONVERSION (optional)                                            |
|      +-- Run virt-v2v to install VirtIO drivers                             |
|                                                                             |
|   7. CREATE VM                                                              |
|      +-- Create KubeVirt VirtualMachine with PVCs attached                  |
|                                                                             |
|   8. CLEANUP                                                                |
|      +-- Delete EBS snapshots (volumes retained by PVCs)                    |
|                                                                             |
+-----------------------------------------------------------------------------+
```

### 2.2 Limitations

#### Same Region Requirement

```
SUPPORTED:
   AWS Account A (us-east-1) -----> OpenShift (us-east-1)
   
   AWS Account A (us-east-1) -----> AWS Account B (us-east-1)  [cross-account]
                                         |
                                         v
                                    OpenShift (us-east-1)

NOT SUPPORTED:
   AWS Account A (us-east-1) --X--> OpenShift (eu-west-1)
   
   Reason: EBS snapshot sharing only works within the same region
```

| Limitation | Reason | Workaround |
|------------|--------|------------|
| Same region only | EBS snapshot sharing is regional | Copy snapshots first (manual) |
| Cold migration only | Snapshot-based transfer | Plan downtime window |
| No instance store | Ephemeral storage | Backup data externally |
| No static IP | Different network model | Update DNS, use services |
| EBS volumes only | Instance store unsupported | Migrate data to EBS first |

### 2.3 Possibilities and Use Cases

#### Supported Scenarios

| Scenario | Description | Example |
|----------|-------------|---------|
| **Same-account** | Source and target in same AWS account | Dev → Production cluster |
| **Cross-account** | Different AWS accounts, same region | Vendor → Customer account |
| **Multi-VM** | Batch migration of multiple VMs | Migrate entire tier |
| **Windows** | Full Windows support with VirtIO drivers | Windows Server 2019/2022 |
| **Linux** | All major distributions | RHEL, Ubuntu, Amazon Linux |

#### Instance Type Mapping

```go
// From pkg/provider/ec2/controller/builder/vm.go
var instanceSizeSpecs = map[string]instanceSizeSpec{
    "nano":     {1, 512},      // 1 vCPU, 512 MiB
    "micro":    {1, 1024},     // 1 vCPU, 1 GiB
    "small":    {1, 2048},     // 1 vCPU, 2 GiB
    "medium":   {2, 4096},     // 2 vCPU, 4 GiB
    "large":    {2, 8192},     // 2 vCPU, 8 GiB
    "xlarge":   {4, 16384},    // 4 vCPU, 16 GiB
    "2xlarge":  {8, 32768},    // 8 vCPU, 32 GiB
    "4xlarge":  {16, 65536},   // 16 vCPU, 64 GiB
    "8xlarge":  {32, 131072},  // 32 vCPU, 128 GiB
    ...
}
```

---

## Part 3: Live Demo

### 3.1 Create EC2 Provider and Show Inventory

#### Step 1: Create the Provider

```bash
# Export credentials
export EC2_KEY="AKIAXXXXXXXXXXXXXXXX"
export EC2_SECRET="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

# Create provider with auto-detected target credentials
kubectl mtv create provider my-ec2 --type ec2 \
  --ec2-region us-east-1 \
  --username "$EC2_KEY" \
  --password "$EC2_SECRET" \
  --auto-target-credentials

# Wait for provider to be ready
kubectl get provider my-ec2 -w
```

#### Step 2: Explore the Inventory

```bash
# List all EC2 instances
kubectl mtv get inventory ec2-instance my-ec2

# Filter stopped instances (ready for migration)
kubectl mtv get inventory ec2-instance my-ec2 -q "where powerState = 'Off'"

# Filter by EC2 tags
kubectl mtv get inventory ec2-instance my-ec2 -q "where label.Environment = 'production'"

# List EBS volumes
kubectl mtv get inventory ec2-volume my-ec2

# List EBS volume types (for storage mapping)
kubectl mtv get inventory ec2-volume-type my-ec2

# List networks (VPCs and Subnets)
kubectl mtv get inventory ec2-network my-ec2
```

### 3.2 Create Migration Plans

#### Option A: With Guest Conversion (Recommended)

```bash
# Create a plan with VirtIO driver installation
kubectl mtv create plan migrate-webserver \
  --source my-ec2 \
  --target host \
  --vms "i-0abc123def456" \
  --target-namespace migrated-vms \
  --default-target-network default \
  --default-target-storage-class gp3-csi

# Start the migration
kubectl mtv start plan migrate-webserver

# Watch progress
kubectl get migration -w
```

#### Option B: Without Conversion (Compatibility Mode)

```bash
# Skip virt-v2v, use SATA/E1000E compatibility devices
kubectl mtv create plan migrate-legacy \
  --source my-ec2 \
  --target host \
  --vms "i-0xyz789abc012" \
  --target-namespace migrated-vms \
  --skip-guest-conversion \
  --use-compatibility-mode
```

**When to use Compatibility Mode:**
- Guest OS already has VirtIO drivers
- Testing/quick migrations
- OS not supported by virt-v2v

#### Option C: Using Query Filters

```bash
# Migrate all VMs with specific tag
kubectl mtv create plan migrate-dev-env \
  --source my-ec2 \
  --target host \
  --vms "where label.tier = 'web' and powerState = 'Off'" \
  --target-namespace dev-vms \
  --default-target-network default \
  --default-target-storage-class gp3-csi
```

### 3.3 Debugging with EC2 Tags

#### Finding Migration Resources in AWS Console

```bash
# Snapshots created during migration
aws ec2 describe-snapshots \
  --filters "Name=tag:forklift.konveyor.io/vmID,Values=i-0abc123def456" \
  --query 'Snapshots[*].[SnapshotId,State,Progress,VolumeSize]' \
  --output table

# Volumes created from snapshots
aws ec2 describe-volumes \
  --filters "Name=tag:forklift.konveyor.io/vmID,Values=i-0abc123def456" \
  --query 'Volumes[*].[VolumeId,State,Size,AvailabilityZone]' \
  --output table
```

#### Tag Reference

| Tag Key | Purpose | Example Value |
|---------|---------|---------------|
| `forklift.konveyor.io/vmID` | Links to source instance | `i-0abc123def456` |
| `forklift.konveyor.io/vm-name` | Human-readable name | `my-web-server` |
| `forklift.konveyor.io/volume` | Source volume ID | `vol-0def456abc` |
| `forklift.konveyor.io/snapshot` | Snapshot ID | `snap-0ghi789jkl` |

### 3.4 Debugging with kubectl/oc

#### Check Migration Status

```bash
# Overall migration status
kubectl get migration -n openshift-mtv

# Detailed plan status
kubectl describe plan migrate-webserver -n openshift-mtv

# VM-specific status
kubectl get migration migrate-webserver -n openshift-mtv -o jsonpath='{.status.vms[*]}' | jq

# Check for errors
kubectl get migration migrate-webserver -n openshift-mtv -o jsonpath='{.status.vms[*].error}'
```

#### Check PVC Status

```bash
# List PVCs created for migration
kubectl get pvc -n migrated-vms -l forklift.konveyor.io/plan=migrate-webserver

# Check PVC binding status
kubectl get pvc -n migrated-vms -o wide

# Check underlying PV
kubectl get pv -l forklift.konveyor.io/plan=migrate-webserver
```

#### Check Conversion Pod (if guest conversion enabled)

```bash
# Find conversion pod
kubectl get pods -n migrated-vms -l forklift.konveyor.io/plan=migrate-webserver

# Check conversion pod logs
kubectl logs -n migrated-vms -l forklift.konveyor.io/plan=migrate-webserver -c virt-v2v

# If pod is failing, check events
kubectl describe pod -n migrated-vms -l forklift.konveyor.io/plan=migrate-webserver
```

#### Check Final VM

```bash
# List migrated VMs
kubectl get vm -n migrated-vms

# Check VM details
kubectl describe vm <vm-name> -n migrated-vms

# Start the VM
kubectl virt start <vm-name> -n migrated-vms

# Console access
kubectl virt console <vm-name> -n migrated-vms
```

---

## Quick Reference

### Essential Commands

```bash
# Provider Management
kubectl mtv create provider <name> --type ec2 --ec2-region <region> --username <key> --password <secret>
kubectl mtv get provider
kubectl mtv delete provider <name>

# Inventory Exploration  
kubectl mtv get inventory ec2-instance <provider>
kubectl mtv get inventory ec2-volume <provider>
kubectl mtv get inventory ec2-network <provider>

# Plan Management
kubectl mtv create plan <name> --source <provider> --target host --vms <vm-ids>
kubectl mtv start plan <name>
kubectl mtv get plan
kubectl mtv describe plan <name>
kubectl mtv cancel plan <name>
kubectl mtv delete plan <name>

# Debugging
kubectl get migration -w
kubectl logs -n <ns> -l forklift.konveyor.io/plan=<plan> -c virt-v2v
kubectl get pvc -n <ns> -l forklift.konveyor.io/plan=<plan>
```

### Required IAM Permissions

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances",
        "ec2:DescribeVolumes", 
        "ec2:DescribeSnapshots",
        "ec2:DescribeSubnets",
        "ec2:DescribeVpcs",
        "ec2:CreateSnapshot",
        "ec2:DeleteSnapshot",
        "ec2:CreateVolume",
        "ec2:DeleteVolume",
        "ec2:CreateTags",
        "ec2:StopInstances",
        "ec2:ModifySnapshotAttribute"
      ],
      "Resource": "*"
    }
  ]
}
```

---

## Q&A

**Common Questions:**

1. **Q: Can I migrate running instances?**
   A: No, instances must be stopped to ensure data consistency (cold migration only).

2. **Q: What happens to my Elastic IPs?**
   A: They remain in EC2. Update DNS or use Kubernetes Services for access.

3. **Q: Can I rollback a migration?**
   A: The source EC2 instance is preserved. You can restart it if migration fails.

4. **Q: How long does migration take?**
   A: Disk transfer is immediate - no data is actually copied (volumes are created directly from snapshots in AWS). The main time is virt-v2v guest conversion which takes a few minutes for OS and disk auto-detection.

5. **Q: Does it support Windows?**
   A: Yes! Both Windows Server and desktop versions with automatic VirtIO driver installation.

---

## Resources

- [EC2 Provider README](../../../pkg/provider/ec2/README.md)
- [Guest Conversion Documentation](../../../pkg/provider/ec2/docs/guest-conversion.md)
- [Resource Tagging Guide](../../../pkg/provider/ec2/docs/resource-tagging.md)
- [Feature Comparison](../../../pkg/provider/ec2/docs/feature-comparison.md)
- [kubectl-mtv Repository](https://github.com/yaacov/kubectl-mtv)
