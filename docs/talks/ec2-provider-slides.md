# EC2 Provider Presentation Slides

---

## Slide 1: Title

# Migrating EC2 Instances to OpenShift Virtualization
## The Forklift EC2 Provider

**Speaker Notes:**
- Welcome everyone to this session on the EC2 provider
- We'll cover AWS EC2 fundamentals, migration architecture, and live demo
- Duration: ~30 minutes with Q&A

---

## Slide 2: Agenda

```
📋 What We'll Cover

1️⃣  AWS EC2 Virtualization (10 min)
    • Instance types: Xen → Nitro → Metal
    • Regions, AZs, and why they matter
    • Storage (EBS) and Networking

2️⃣  Migration Architecture (10 min)
    • The migration pipeline
    • Limitations and possibilities

3️⃣  Live Demo (10 min)
    • kubectl-mtv walkthrough
    • Debugging tips
```

**Speaker Notes:**
- We'll go deep enough to understand the "why" behind the design
- The demo will be hands-on with real commands you can use

---

## Slide 3: EC2 Instance Type Evolution

```
                    EC2 Virtualization Timeline
                    
    2006              2017                2018+
     │                 │                   │
     ▼                 ▼                   ▼
  ┌──────┐         ┌───────┐          ┌───────┐
  │ Xen  │   →     │ Nitro │    →     │ Metal │
  └──────┘         └───────┘          └───────┘
     │                 │                   │
  Software         Hardware              No
  Hypervisor       Hypervisor         Hypervisor
     │                 │                   │
  xen-blk/net      NVMe/ENA            Bare
  drivers          drivers             Metal
```

**Speaker Notes:**
- Xen: Original virtualization, paravirtual drivers (xen-blkfront, xen-netfront)
- Nitro: Custom AWS silicon, hardware-offloaded hypervisor, near-bare-metal performance
- Metal: No hypervisor at all! Perfect for running KubeVirt/nested virtualization
- Key point: All these driver types need conversion to VirtIO for KubeVirt

---

## Slide 4: Why Drivers Matter

```
┌────────────────────────────────────────────────────────────┐
│                Driver Conversion                           │
├────────────────────────────────────────────────────────────┤
│                                                            │
│   EC2 Instance              KubeVirt VM                    │
│   ┌──────────────┐          ┌──────────────┐              │
│   │ Xen/Nitro    │   →      │   VirtIO     │              │
│   │ xen-blkfront │   →      │   virtio-blk │              │
│   │ xen-netfront │   →      │   virtio-net │              │
│   │ ENA driver   │   →      │   virtio-net │              │
│   │ NVMe driver  │   →      │   virtio-blk │              │
│   └──────────────┘          └──────────────┘              │
│                                                            │
│   ⚡ virt-v2v handles this automatically!                  │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

**Speaker Notes:**
- This is why we need guest conversion (virt-v2v)
- virt-v2v detects the OS and installs appropriate VirtIO drivers
- Also removes AWS-specific agents (cloud-init EC2 datasource, SSM agent, etc.)

---

## Slide 5: Regions and Availability Zones

```
           AWS Region: us-east-1 (N. Virginia)
┌───────────────────────────────────────────────────────────┐
│                                                           │
│   ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌─────────┐ │
│   │  AZ-a   │   │  AZ-b   │   │  AZ-c   │   │  AZ-d   │ │
│   │         │   │         │   │         │   │         │ │
│   │ ┌─────┐ │   │         │   │         │   │         │ │
│   │ │ EC2 │ │   │         │   │         │   │         │ │
│   │ └──┬──┘ │   │         │   │         │   │         │ │
│   │    │    │   │         │   │         │   │         │ │
│   │ ┌──▼──┐ │   │         │   │         │   │         │ │
│   │ │ EBS │ │   │         │   │ ┌─────┐ │   │         │ │
│   │ └─────┘ │   │         │   │ │ OCP │ │   │         │ │
│   └─────────┘   └─────────┘   │ │Nodes│ │   └─────────┘ │
│                               │ └─────┘ │               │
│                               └─────────┘               │
│                                                           │
│   ⚠️  EBS volumes CANNOT move between AZs!               │
│   📸 Snapshots CAN cross AZs (this is our solution!)     │
│                                                           │
└───────────────────────────────────────────────────────────┘
```

**Speaker Notes:**
- Critical concept: EBS volumes are AZ-locked
- If OpenShift nodes are in us-east-1c, volumes must be created there
- We solve this with snapshots: Create in source AZ → Snapshot → New volume in target AZ
- Snapshots are region-wide (not AZ-specific) - this enables cross-AZ migration
- The `target-az` provider setting controls where new volumes are created

---

## Slide 6: AWS Storage Types

```
              AWS Storage: EBS vs S3

┌──────────────────────────────────────────────────────────┐
│                                                          │
│   EBS (Block Storage)          S3 (Object Storage)      │
│   ┌────────────────────┐       ┌────────────────────┐  │
│   │ • VM disks         │       │ • Files, backups   │  │
│   │ • AZ-specific      │       │ • Regional         │  │
│   │ • Attached to EC2  │       │ • HTTP access      │  │
│   └────────────────────┘       └────────────────────┘  │
│                                                          │
│   📸 EBS Snapshots are stored in AWS-owned S3           │
│      (not your buckets - managed by AWS internally)     │
│      This is why snapshots are REGIONAL!                │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**Speaker Notes:**
- AWS has two main storage services: EBS (block) and S3 (object)
- EBS volumes are AZ-specific, S3 is regional
- EBS snapshots are stored in AWS-owned S3 - this is why they're region-wide
- You don't see snapshots in your S3 console - AWS manages this internally

---

## Slide 7: EBS Volume Types

```
              EBS Volume Types - All Supported!

┌──────────────────────────────────────────────────────────┐
│                                                          │
│   General Purpose SSD      Provisioned IOPS SSD         │
│   ┌────────┬────────┐      ┌────────┬────────┐         │
│   │  gp2   │  gp3   │      │  io1   │  io2   │         │
│   │ 3K IOPS│16K IOPS│      │64K IOPS│256K IOP│         │
│   └────────┴────────┘      └────────┴────────┘         │
│                                                          │
│   ⚠️  Volumes STAY as EBS after migration!              │
│   ⚠️  StorageClass MUST use ebs.csi.aws.com driver!    │
│                                                          │
│   ❌ Instance Store (ephemeral) - NOT SUPPORTED         │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**Speaker Notes:**
- All EBS volume types supported (gp2, gp3, io1, io2, st1, sc1)
- KEY POINT: Volumes STAY as EBS - we don't copy data to other storage!
- The EBS CSI driver is HARDCODED in the PV spec (`ebs.csi.aws.com`)
- StorageClass must use EBS CSI provisioner - can't use ODF, local-path, etc.
- Instance store is ephemeral and cannot be snapshotted - data will be lost

---

## Slide 8: EC2 Networking - VPC and Subnets

```
              VPC and Subnet Architecture

┌──────────────────────────────────────────────────────────┐
│   VPC: 10.0.0.0/16 (Regional - spans all AZs)           │
│                                                          │
│   ┌─────────────────┐     ┌─────────────────┐          │
│   │ Subnet 1a       │     │ Subnet 1c       │          │
│   │ 10.0.1.0/24     │     │ 10.0.3.0/24     │          │
│   │ (us-east-1a)    │     │ (us-east-1c)    │          │
│   │                 │     │                 │          │
│   │ ┌─────────────┐ │     │                 │          │
│   │ │ EC2 + ENI   │ │     │                 │          │
│   │ │ 10.0.1.50   │ │     │                 │          │
│   │ └─────────────┘ │     │                 │          │
│   └─────────────────┘     └─────────────────┘          │
│                                                          │
│   VPC = Regional,  Subnets = AZ-specific                │
└──────────────────────────────────────────────────────────┘
```

**Speaker Notes:**
- VPC (Virtual Private Cloud) is your isolated network - spans all AZs in a region
- Subnets are subdivisions of VPC - each subnet lives in ONE specific AZ
- **CIDR notation:** `10.0.0.0/16` means 16 bits for network = 65,536 addresses; `/24` = 256 addresses
- ENI (Elastic Network Interface) is the virtual NIC attached to instances
- We map SUBNETS (not VPCs) to target networks

---

## Slide 9: Network Mapping Options

```
              EC2 Subnet → OpenShift Network

┌──────────────────────────────────────────────────────────┐
│                                                          │
│   EC2 Subnet              Target Type                   │
│   ┌──────────────┐                                      │
│   │ subnet-xxx   │ ──►  1. pod (default)               │
│   │              │         └─ Cluster SDN, masquerade   │
│   │              │         └─ With UDN: l2bridge        │
│   │              │                                      │
│   │              │ ──►  2. multus                       │
│   │              │         └─ Bridge to external L2     │
│   │              │         └─ Specify NAD name          │
│   │              │                                      │
│   │              │ ──►  3. ignored                      │
│   └──────────────┘         └─ Skip this interface       │
│                                                          │
│   ✅ MAC addresses preserved (with UDN enabled)         │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**Speaker Notes:**
- Three destination types: `pod`, `multus`, `ignored`
- **pod**: Uses cluster networking (masquerade NAT)
- **multus**: Bridge to external networks via NetworkAttachmentDefinition
- **ignored**: Skip interface entirely
- **UDN** is a cluster-level feature (OCP 4.15+), not a mapping type
  - When UDN enabled, `pod` type automatically uses `l2bridge` binding
  - Better MAC preservation support

---

## Slide 10: Migration Pipeline

```
                EC2 Migration Pipeline

     ┌─────────────────────────────────────────────┐
     │                                             │
  1. │  📋 Initialize                              │
     │     └─ Validate VM, create tracking         │
     │                                             │
  2. │  ⏹️  Prepare Source                         │
     │     └─ Stop EC2 instance                    │
     │                                             │
  3. │  📸 Create Snapshots                        │
     │     └─ Snapshot all EBS volumes             │
     │     └─ Tag with forklift.konveyor.io/*     │
     │                                             │
  4. │  🔗 Share Snapshots (cross-account only)   │
     │     └─ Share with target AWS account        │
     │                                             │
  5. │  💾 Create Volumes → PVs → PVCs            │
     │     └─ New volumes in target AZ             │
     │     └─ CSI volumeHandle binding             │
     │                                             │
  6. │  🔧 Convert Guest (optional)                │
     │     └─ virt-v2v installs VirtIO drivers     │
     │                                             │
  7. │  🖥️  Create VM                              │
     │     └─ KubeVirt VirtualMachine              │
     │                                             │
  8. │  🧹 Cleanup                                 │
     │     └─ Delete snapshots                     │
     │                                             │
     └─────────────────────────────────────────────┘
```

**Speaker Notes:**
- Cold migration only - instance must be stopped
- Snapshots are region-wide, enabling cross-AZ volume creation
- CreateVolume API lets you specify any AZ in the region when restoring from snapshot
- Guest conversion is optional but recommended
- Cleanup preserves volumes (now backing PVCs) but removes snapshots

---

## Slide 11: Limitations

```
              EC2 Provider Limitations

┌───────────────────────────────────────────────────────────┐
│                                                           │
│   ❌ Cross-Region Migration                               │
│      Snapshot sharing is regional only                    │
│      Workaround: Copy snapshot to target region first     │
│                                                           │
│   ❌ Warm/Live Migration                                  │
│      Snapshot-based = cold migration only                 │
│      Plan for downtime window                             │
│                                                           │
│   ❌ Instance Store Volumes                               │
│      Ephemeral storage cannot be snapshotted             │
│      Migrate data to EBS first                           │
│                                                           │
│   ❌ Static IP Preservation                               │
│      Different network model in Kubernetes               │
│      Use Services/DNS for stable endpoints               │
│                                                           │
└───────────────────────────────────────────────────────────┘
```

**Speaker Notes:**
- Same region required: Snapshots are regional, can't be used across regions
- Cross-region would need `CopySnapshot` API (not currently supported)
- Cold migration = plan maintenance window
- Static IPs don't make sense in K8s - use Services instead
- Instance store: backup data externally or migrate to EBS

---

## Slide 12: Demo - Create Provider

```bash
# Create EC2 Provider
kubectl mtv create provider my-ec2 --type ec2 \
  --ec2-region us-east-1 \
  --username "$EC2_KEY" \
  --password "$EC2_SECRET" \
  --auto-target-credentials

# What --auto-target-credentials does:
# 1. Fetches target AWS creds from kube-system/aws-creds
# 2. Detects target AZ from worker node topology labels
```

**Speaker Notes:**
- The `--auto-target-credentials` flag is a time-saver
- It reads the cluster's AWS credentials (same account as ROSA/OCP)
- Also auto-detects which AZ your worker nodes are in

---

## Slide 13: Demo - Explore Inventory

```bash
# List all EC2 instances
kubectl mtv get inventory ec2-instance my-ec2

# Filter with TreeQL queries
kubectl mtv get inventory ec2-instance my-ec2 \
  -q "where powerState = 'Off'"

# Filter by EC2 tags!
kubectl mtv get inventory ec2-instance my-ec2 \
  -q "where label.Environment = 'production'"

# Other inventory types
kubectl mtv get inventory ec2-volume my-ec2
kubectl mtv get inventory ec2-volume-type my-ec2
kubectl mtv get inventory ec2-network my-ec2
```

**Speaker Notes:**
- EC2 tags are exposed as labels in the inventory
- This enables powerful filtering based on existing tagging strategy
- Stopped instances are ideal migration candidates
- Volume types help plan storage mapping

---

## Slide 14: Demo - Create and Start Plan

```bash
# Create migration plan
kubectl mtv create plan migrate-webserver \
  --source my-ec2 \
  --target host \
  --vms "i-0abc123def456" \
  --target-namespace migrated-vms \
  --default-target-network default \
  --default-target-storage-class gp3-csi

# Or with query filter:
kubectl mtv create plan migrate-web-tier \
  --source my-ec2 \
  --target host \
  --vms "where label.tier = 'web'" \
  --target-namespace web-vms

# Start the migration
kubectl mtv start plan migrate-webserver
```

**Speaker Notes:**
- `--target host` means the local cluster (host provider)
- Can specify VM IDs or use query filters
- Network and storage mappings can be explicit or default
- Starting the plan creates a Migration CR

---

## Slide 15: Demo - Compatibility Mode

```bash
# Skip virt-v2v, use compatibility devices
kubectl mtv create plan migrate-legacy \
  --source my-ec2 \
  --target host \
  --vms "i-0xyz789abc012" \
  --skip-guest-conversion \
  --use-compatibility-mode
```

```
Compatibility Mode Uses:
┌──────────────┬──────────────────┐
│   Standard   │  Compatibility   │
├──────────────┼──────────────────┤
│ virtio-blk   │     SATA         │
│ virtio-net   │     E1000E       │
│ virtio-input │     USB          │
└──────────────┴──────────────────┘
```

**Speaker Notes:**
- Use when guest already has VirtIO drivers
- Or for quick testing without conversion overhead
- Compatibility devices work on most OSes but lower performance

---

## Slide 16: Debugging - AWS Tags

```bash
# Find snapshots created by Forklift
aws ec2 describe-snapshots \
  --filters "Name=tag:forklift.konveyor.io/vmID,Values=i-0abc123" \
  --output table

# Find created volumes
aws ec2 describe-volumes \
  --filters "Name=tag:forklift.konveyor.io/vmID,Values=i-0abc123" \
  --output table
```

```
Tags Applied by Forklift:
┌────────────────────────────────┬─────────────────────┐
│           Tag Key              │    Example Value    │
├────────────────────────────────┼─────────────────────┤
│ forklift.konveyor.io/vmID      │ i-0abc123def456     │
│ forklift.konveyor.io/vm-name   │ my-web-server       │
│ forklift.konveyor.io/volume    │ vol-0def456abc      │
│ forklift.konveyor.io/snapshot  │ snap-0ghi789jkl     │
└────────────────────────────────┴─────────────────────┘
```

**Speaker Notes:**
- Tags enable recovery after controller restarts
- Can manually clean up orphaned resources by tag
- Visible in AWS Console for troubleshooting

---

## Slide 17: Debugging - Kubernetes Side

```bash
# Watch migration progress
kubectl get migration -w

# Detailed status
kubectl describe plan migrate-webserver

# Check PVCs
kubectl get pvc -n migrated-vms \
  -l forklift.konveyor.io/plan=migrate-webserver

# Conversion pod logs
kubectl logs -n migrated-vms \
  -l forklift.konveyor.io/plan=migrate-webserver \
  -c virt-v2v

# Final VM
kubectl get vm -n migrated-vms
kubectl virt start <vm-name>
kubectl virt console <vm-name>
```

**Speaker Notes:**
- Migration CR shows overall progress and errors
- PVC status shows disk provisioning state
- Conversion pod logs show virt-v2v output
- Use `kubectl virt` commands to manage final VM

---

## Slide 18: Summary

```
              EC2 Provider - Key Takeaways

  ✅ Supports all EBS volume types
  ✅ Same-account and cross-account migrations
  ✅ Automatic VirtIO driver installation
  ✅ AWS tag-based filtering and tracking
  ✅ AZ-aware volume placement
  
  ⚠️  Cold migration only (plan downtime)
  ⚠️  Same region required
  ⚠️  No instance store support
  
  🛠️ kubectl-mtv makes it easy!
```

**Speaker Notes:**
- EC2 provider brings AWS VMs into the OpenShift ecosystem
- Designed for ROSA and OCP-on-AWS environments
- kubectl-mtv provides a great user experience
- Questions?

---

## Slide 19: Resources

```
📚 Documentation

• EC2 Provider README
  pkg/provider/ec2/README.md

• Guest Conversion Guide  
  pkg/provider/ec2/docs/guest-conversion.md

• Resource Tagging Guide
  pkg/provider/ec2/docs/resource-tagging.md

• kubectl-mtv
  https://github.com/yaacov/kubectl-mtv

🔗 Forklift Project
   https://github.com/kubev2v/forklift
```

---

## Slide 20: Q&A

# Questions?

```
  Common Questions:

  Q: Can I migrate running instances?
  A: No, must be stopped for data consistency.

  Q: What about Elastic IPs?
  A: They stay in EC2. Use K8s Services instead.

  Q: How long does migration take?
  A: ~1-2 min per 100GB for snapshots, plus conversion.

  Q: Windows support?
  A: Yes! Full support with automatic driver install.
```

---
