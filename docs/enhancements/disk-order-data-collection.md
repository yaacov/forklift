---
title: disk-order-data-collection
authors:
  - "@yaacov"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2026-03-18
last-updated: 2026-03-18
status: implementable
---

# Collect Disk Order Data from All Providers

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] User-facing documentation is created

## Summary

Forklift needs deterministic disk ordering when building target VMs to ensure
the boot disk and data disks appear in the correct order. Today, only three
providers (vSphere, EC2, OCP) collect enough data and set `AnnDiskIndex`.
The remaining four providers (oVirt, OpenStack, HyperV, OVA/OVF) either drop
available topology fields during inventory collection or never collect them at
all, resulting in undefined disk order on the target.

This enhancement collects the missing disk topology and ordering fields
from every source provider into the inventory model. A follow-up enhancement
will use the collected data to implement correct sorting and set `AnnDiskIndex`
for all providers.

### Goals

* Collect all source-available disk ordering data into the Forklift inventory
  model for every provider.
* No provider should silently drop fields that are needed for disk ordering.

### Non-Goals

* This enhancement does **not** change disk sorting logic or set `AnnDiskIndex`
  on providers that currently lack it. That is a follow-up.
* This enhancement does not change the target VM build path.

## Provider Reference

| Provider | Collected Now | Added by This Enhancement | Disk Order Now | Future Disk Order (follow-up) |
|----------|--------------|---------------------------|----------------|-------------------------------|
| vSphere | Bus, Key, ControllerKey, UnitNumber on Disk; Key, Bus on Controller | `BusNumber` on Controller | Bus priority + Key; `AnnDiskIndex` set | Bus → BusNumber → UnitNumber |
| EC2 | DeviceName, EBS.VolumeId from BlockDeviceMappings | Nothing (already complete) | BlockDeviceMappings order; `AnnDiskIndex` set | No change |
| OCP | VM export volume list (preserves source spec) | Nothing (already complete) | vmExport volume order; `AnnDiskIndex` set | No change |
| oVirt | ID, Interface, Bootable, SCSIReservation, Disk ID | `LogicalName` on DiskAttachment | Raw API order; `AnnDiskIndex` **not** set | Sort by LogicalName |
| OpenStack | Volume Attachment ID only | `Device`, `ServerID`, `VolumeID` on Attachment | AttachedVolumes iteration order; `AnnDiskIndex` **not** set | Sort by Device |
| HyperV | ID, WindowsPath, SMBPath, Capacity, Format, RCTEnabled | `ControllerType`, `ControllerNum`, `ControllerLoc` | extractDisks() iteration order; `AnnDiskIndex` **not** set | ControllerType → Num → Loc |
| OVA/OVF | DiskSection: Capacity, Format, DiskId, FileRef, PopulatedSize | `ControllerType`, `ControllerAddress`, `AddressOnParent` | DiskSection array order; `AnnDiskIndex` **not** set | ControllerType → Address → AddressOnParent |

### Per-Provider Details

#### vSphere

* **Source API:** VMware `VirtualDevice` (Key, ControllerKey, UnitNumber) and `VirtualController` (BusNumber, Bus type).
* **Collected now:** Bus (SCSI/SATA/IDE/NVME), Key, ControllerKey, UnitNumber on each Disk; Key and Bus on each Controller.
* **Enhancement:** Add `BusNumber` to Controller model (from `VirtualController.BusNumber` -- distinguishes SCSI0 vs SCSI1).
* **Current order:** Sorted by hardcoded bus priority then Key. `AnnDiskIndex` is set.
* **Future order:** Bus → BusNumber → UnitNumber. Matches BIOS/libvirt enumeration.

#### EC2

* **Source API:** `BlockDeviceMappings` with `DeviceName` and `EBS.VolumeId`.
* **Collected now:** All fields.
* **Enhancement:** None -- already complete.
* **Current order:** BlockDeviceMappings API order. `AnnDiskIndex` is set.
* **Future order:** No change needed.

#### OCP (OpenShift)

* **Source API:** VM export (`VirtualMachineExport`).
* **Collected now:** VM export volume list preserves source VM spec disk order.
* **Enhancement:** None -- already complete.
* **Current order:** vmExport volume order. `AnnDiskIndex` is set.
* **Future order:** No change needed.

#### oVirt

* **Source API:** oVirt REST API `disk_attachments` with `logical_name` field.
* **Collected now:** DiskAttachment ID, Interface, Bootable, SCSIReservation, Disk ID.
* **Enhancement:** Add `LogicalName` to DiskAttachment model (e.g. `/dev/vda`). Requires running VM with guest agent; empty when VM is off.
* **Current order:** Unordered (API iteration). `AnnDiskIndex` not set.
* **Future order:** Sort by LogicalName alphabetically. Fallback to API order when empty.

#### OpenStack

* **Source API:** Cinder Volume API (`Volume.Attachments`) and Nova (`os-extended-volumes:volumes_attached`).
* **Collected now:** Volume Attachment ID only.
* **Enhancement:** Populate `Device` (e.g. `/dev/vda`), `ServerID`, `VolumeID` on Cinder Attachment. Model fields already exist but were never populated.
* **Current order:** Unordered (AttachedVolumes iteration). `AnnDiskIndex` not set.
* **Future order:** Match Attachment to VM via ServerID, sort by Device. Image root disk gets index 0.

#### HyperV

* **Source API:** PowerShell `Get-VMHardDiskDrive` via WinRM.
* **Collected now:** Disk ID, WindowsPath, SMBPath, Capacity, Format, RCTEnabled.
* **Enhancement:** Add `ControllerType` (IDE/SCSI), `ControllerNum`, `ControllerLoc`. Data is already parsed by WinRM driver `DiskInfo` but dropped by `extractDisks()`.
* **Current order:** Unordered (iteration order). `AnnDiskIndex` not set.
* **Future order:** ControllerType (IDE < SCSI) → ControllerNum → ControllerLoc.

#### OVA/OVF

* **Source API:** OVF `VirtualHardwareSection` Items (controllers: ResourceType 5/6/20; disks: ResourceType 17 with Parent, HostResource, AddressOnParent).
* **Collected now:** DiskSection only (Capacity, Format, DiskId, FileRef, PopulatedSize). HardDiskDrive Items discarded.
* **Enhancement:** Add `AddressOnParent` to OVF Item struct. Extract controller topology from VirtualHardwareSection. Add `ControllerType`, `ControllerAddress`, `AddressOnParent` to VmDisk and model.Disk.
* **Current order:** DiskSection array order. `AnnDiskIndex` not set.
* **Future order:** ControllerType bus priority (SCSI, SATA, IDE, NVME) → ControllerAddress → AddressOnParent.

## Proposal

This enhancement adds missing disk topology fields to the inventory model for
each provider. The changes are collection-only -- no sorting logic, no
`AnnDiskIndex` changes, no target VM build changes. The data will be consumed
by a follow-up enhancement that implements deterministic disk ordering.

### Security, Risks, and Mitigations

No new security risks. The additional fields are read-only topology metadata
from the source provider APIs. No new credentials or permissions are required.

**Risks:**
* **oVirt `logicalName` empty when VM is off:** The oVirt API only populates
  `logical_name` when the VM is running with guest agent active. The follow-up
  sorting will need a fallback.
* **OVA without controller Items:** Some minimal OVFs lack controller topology.
  Fields will be zero-valued; follow-up falls back to DiskSection order.

## Design Details

### Test Plan

* Unit tests verifying the new fields are populated correctly from source data
  for each provider (oVirt, OpenStack, HyperV, OVA/OVF, vSphere).
* Verify that existing tests continue to pass (no behavioral change).

### Upgrade / Downgrade Strategy

New fields are additive to the inventory model. Existing plans and migrations
are unaffected. On downgrade, the new fields are simply absent from the model
and ignored.

## Implementation History

* 03/18/2026 - Enhancement submitted.
