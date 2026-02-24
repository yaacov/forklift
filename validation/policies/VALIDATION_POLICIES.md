# Forklift Validation Policies

This document catalogs all Rego-based validation policies used by the Migration Toolkit for Virtualization (Forklift/MTV) to assess VM migration readiness. Each policy checks for a specific known issue or unsupported configuration and raises a **concern** with one of three severity levels:

- **Critical** — Migration blocker. The VM cannot be migrated until the issue is resolved.
- **Warning** — The VM can be migrated, but some functionality will be lost or require manual reconfiguration post-migration.
- **Information** — Awareness only. A difference exists between source and target environments.

---

## Summary

| Provider | Critical | Warning | Information | Total |
|----------|----------|---------|-------------|-------|
| VMware   | 6        | 13      | 6           | 25    |
| oVirt    | 5        | 27      | 6           | 38    |
| OpenStack| 3        | 10      | 2           | 15    |
| OVA      | 1        | 4       | 0           | 5     |
| **Total**| **15**   | **54**  | **14**      | **83**|

---

## VMware Provider

Policy directory: `validation/policies/io/konveyor/forklift/vmware/`

### Critical

| Rego File | Concern ID | Label | Description |
|-----------|-----------|-------|-------------|
| `datastore.rego` | `vmware.datastore.missing` | Disk is not located on a datastore | Checks if any VM disk has a null or empty datastore ID. A disk without a backing datastore cannot be migrated. |
| `disk_mode.rego` | `vmware.disk_mode.independent` | Independent disk detected | Detects disks in `independent_persistent` or `independent_nonpersistent` mode. Independent disks cannot be transferred via VDDK and must be changed to "Dependent" mode before migration. |
| `disk_size.rego` | `vmware.disk.capacity.invalid` | Disk has an invalid capacity | Flags any disk with zero or negative capacity (in bytes). Such disks are invalid and cannot be migrated. |
| `filesystem_size.rego` | `vmware.guestDisks.freespace` | Insufficient free space for conversion | Validates that guest filesystems have enough free space for the virt-v2v conversion process: 100 MB for `/` and `C:\`, 50 MB for `/boot`, and 10 MB for all other mountpoints. |
| `passthrough_device.rego` | `vmware.passthrough_device.detected` | Passthrough device detected | Detects PCI passthrough devices (`VirtualPCIPassthrough`). The VM **cannot** be migrated until the passthrough device is removed. |
| `rdm_disk.rego` | `vmware.disk.rdm.detected` | Raw Device Mapped disk detected | Detects Raw Device Mapped (RDM) disks. The VM **cannot** be migrated unless RDM disks are removed. They can be reattached after migration. |

### Warning

| Rego File | Concern ID | Label | Description |
|-----------|-----------|-------|-------------|
| `changed_block_tracking.rego` | `vmware.changed_block_tracking.disabled` | Changed Block Tracking (CBT) not enabled | Checks if CBT is disabled at the VM level. CBT is a prerequisite for warm migration. Without it, only cold migration is available. |
| `changed_block_tracking_per_disk.rego` | `vmware.changed_block_tracking.disk.disabled` | Disk does not have CBT enabled | Checks each individual disk for CBT enablement. Each disk without CBT is flagged separately as it cannot participate in warm migration. |
| `cpu_affinity.rego` | `vmware.cpu_affinity.detected` | CPU affinity detected | Detects VMs with vCPU-to-pCPU pinning. The VM migrates without this setting; administrators can reconfigure it post-migration. |
| `cpu_memory_hotplug.rego` | `vmware.cpu_memory.hotplug.enabled` | CPU/Memory hotplug detected | Flags VMs with CPU hot-add, CPU hot-remove, or memory hot-add enabled. These features are unsupported in OpenShift Virtualization. |
| `host_affinity.rego` | `vmware.host_affinity.detected` | VM-Host affinity detected | Detects VMs bound to specific hosts via affinity rules. The VM migrates without node affinity; it can be set post-migration. |
| `hostname.rego` | `vmware.hostname.empty` | Empty Host Name | Flags VMs with a missing or empty hostname. The hostname may be renamed during migration. |
| `hostname.rego` | `vmware.hostname.default` | Default Host Name | Flags VMs using the default hostname `localhost.localdomain`. The hostname may be renamed during migration. |
| `ip_state.rego` | `vmware.vm_missing_ip.detected` | VM is missing IP addresses | Detects VMs with no IP addresses reported in `guestNetworks`. Static IP preservation requires the VM to be powered on and running VMware Tools. |
| `name.rego` | `vmware.vm.name.invalid` | Invalid VM Name | Validates the VM name against RFC 1123 DNS subdomain rules (max 63 characters, lowercase alphanumeric, hyphens, and periods). Non-compliant names are auto-renamed during migration. |
| `numa_affinity.rego` | `vmware.numa_affinity.detected` | NUMA node affinity detected | Detects NUMA node affinity configuration. This setting is not preserved during migration. |
| `sriov_device.rego` | `vmware.device.sriov.detected` | SR-IOV passthrough adapter detected | Detects SR-IOV passthrough network adapters (`VirtualSriovEthernetCard`). The VM migrates but without SR-IOV; administrators can configure it post-migration. |
| `tpm_enabled.rego` | `vmware.tpm.detected` | TPM detected | Detects VMs with a Trusted Platform Module (TPM) device enabled. TPM data is **not** transferred during migration. |
| `usb_controller.rego` | `vmware.usb_controller.detected` | USB controller detected | Detects USB controllers (`VirtualUSBController`). The VM can migrate but USB-attached devices will not transfer. |
| `vm_os.rego` | `vmware.os.unsupported` | Unsupported operating system detected | Validates the guest OS against a supported list: RHEL 7–10, Windows 10/11, and Windows Server 2016–2025. Unsupported OSes are flagged. |

### Information

| Rego File | Concern ID | Label | Description |
|-----------|-----------|-------|-------------|
| `disk_serial_numbers.rego` | `vmware.disk_serial.truncated` | Disk serial numbers may be truncated | If `disk.EnableUUID` is TRUE and the VM has SCSI disks, warns that disk serial numbers will be truncated after migration. Relevant for applications relying on consistent SCSI serial numbers. |
| `dpm_enabled.rego` | `vmware.dpm.enabled` | vSphere DPM detected | Detects Distributed Power Management on the host cluster. DPM is not available in OpenShift Virtualization. |
| `drs_enabled.rego` | `vmware.drs.enabled` | VM running in a DRS-enabled cluster | Detects Distributed Resource Scheduling on the host cluster. DRS is not available in the target environment. |
| `fault_tolerance.rego` | `vmware.fault_tolerance.enabled` | Fault tolerance | Flags VMs with VMware Fault Tolerance enabled. This feature is not supported in OpenShift Virtualization. |
| `guest_disk_mapping.rego` | `vmware.guestDisks.key.not_found` | Missing disk key mapping | For Windows VMs, checks whether guest disk-to-VMDK key mappings exist. Missing mappings mean `winDriveLetter` cannot be resolved in PVC name templates. |
| `snapshot.rego` | `vmware.snapshot.detected` | VM snapshot detected | Detects VM snapshots. Online snapshots are unsupported in OpenShift Virtualization; the VM migrates with its current snapshot state. |

---

## oVirt / RHV Provider

Policy directory: `validation/policies/io/konveyor/forklift/ovirt/`

### Critical

| Rego File | Concern ID | Label | Description |
|-----------|-----------|-------|-------------|
| `disk_size.rego` | `ovirt.disk.capacity.invalid` | Disk has an invalid capacity | Flags disks with invalid capacity: zero/negative `provisionedSize` for image-type disks, or missing/invalid LUN size for LUN-type disks. |
| `disk_status.rego` | `ovirt.disk.illegal_or_locked_status` | Illegal or locked disk status | Detects disks in `illegal` or `locked` status. Disk transfer will likely fail. |
| `disk_storage_type.rego` | `ovirt.disk.storage_type.unsupported` | Unsupported disk storage type detected | Validates that each disk's storage type is either `image` or `lun`. Any other type (e.g. `cinder`) is unsupported and will likely cause transfer failure. **This is the closest existing validation to "storage array type."** |
| `illegal_images.rego` | `ovirt.disk.illegal_images.detected` | Illegal disk images detected | Detects snapshots containing disks in ILLEGAL state. Disk transfer will likely fail. |
| `vm_status.rego` | `ovirt.vm.status_invalid` | Invalid VM status | Validates the VM status is `up` or `down`. Any other status (e.g. `migrating`, `saving_state`) may cause migration failure. |

### Warning

| Rego File | Concern ID | Label | Description |
|-----------|-----------|-------|-------------|
| `bios_boot_menu.rego` | `ovirt.bios.boot_menu.enabled` | BIOS boot menu enabled | Detects VMs with BIOS boot menu enabled. Unsupported in target; the VM migrates without it. |
| `cpu_custom_model.rego` | `ovirt.cpu.custom_model.detected` | Custom CPU Model detected | Flags VMs configured with a custom CPU model. The configuration transfers but may not be supported in OpenShift Virtualization. |
| `cpu_policy.rego` | `ovirt.cpu.pinning_policy.unsupported` | Unsupported CPU pinning policy | Detects `resize_and_pin_numa` or `isolate_threads` CPU pinning policies, which are not supported in OpenShift Virtualization. |
| `cpu_shares.rego` | `ovirt.cpu.shares.defined` | CPU Shares Defined | Flags VMs with CPU shares configured. This feature is not available in the target environment. |
| `cpu_tune.rego` | `ovirt.cpu.tuning.detected` | CPU tuning detected | Detects custom CPU affinity (beyond 1:1 vCPU-pCPU mapping). Unsupported in OpenShift Virtualization. |
| `custom_properties.rego` | `ovirt.vm.custom_properties.detected` | VM custom properties detected | Flags VMs with custom properties, which have no equivalent in OpenShift Virtualization. |
| `disk_interface_type.rego` | `ovirt.disk.interface_type.unsupported` | Unsupported disk interface type | Validates disk interface types. Only `sata`, `virtio_scsi`, and `virtio` are supported; other types are converted to `virtio`. |
| `ha.rego` | `ovirt.ha.enabled` | VM configured as HA | Detects high-availability configuration. HA is not supported in OpenShift Virtualization. |
| `ha_reservation.rego` | `ovirt.ha.reservation.enabled` | Cluster has HA reservation | Detects clusters with HA resource reservation enabled. Unsupported in the target environment. |
| `host_devices.rego` | `ovirt.host_devices.mapped` | VM has mapped host devices | Flags VMs with host device passthrough. Devices will not be attached post-migration. |
| `ksm.rego` | `ovirt.cluster.ksm_enabled` | Cluster has KSM enabled | Detects Kernel Same-page Merging (KSM) on the host cluster. This memory optimization feature is unsupported in the target. |
| `name.rego` | `ovirt.name.invalid` | Invalid VM Name | Validates VM name against RFC 1123 (max 63 chars, DNS-compatible format). Non-compliant names are auto-renamed. |
| `nic_custom_properties.rego` | `ovirt.nic.custom_properties.detected` | vNIC custom properties detected | Detects vNIC profiles with custom properties, which are unsupported. |
| `nic_interface_type.rego` | `ovirt.nic.interface_type.unsupported` | Unsupported NIC interface type | Validates NIC interfaces. Only `e1000`, `rtl8139`, and `virtio` are supported; others convert to `virtio`. |
| `nic_network_filter.rego` | `ovirt.nic.network_filter.detected` | NIC with network filter detected | Flags vNIC profiles configured with network filters. Unsupported in OpenShift Virtualization. |
| `nic_pci_passthrough.rego` | `ovirt.nic.pci_passthrough.detected` | NIC with host device passthrough | Detects NIC host-device passthrough. VM gets an SR-IOV NIC, but destination network setup is required. |
| `nic_plugged.rego` | `ovirt.nic.unplugged.detected` | Unplugged NIC detected | Detects NICs that are unplugged from a network. Unsupported in OpenShift Virtualization. |
| `nic_port_mirroring.rego` | `ovirt.nic.port_mirroring.detected` | NIC with port mirroring detected | Flags vNIC profiles with port mirroring enabled. Unsupported in the target. |
| `nic_qos.rego` | `ovirt.nic.qos.detected` | NIC with QoS settings detected | Flags vNIC profiles with Quality of Service settings. Unsupported in the target. |
| `numa_tune.rego` | `ovirt.numa.tuning.detected` | NUMA tuning detected | Detects NUMA affinity configuration. Not preserved during migration. |
| `online_snapshot.rego` | `ovirt.snapshot.online_memory.detected` | Online (memory) snapshot detected | Detects snapshots that include a memory copy. Online snapshots are unsupported. |
| `placement_policy.rego` | `ovirt.placement_policy.affinity_set` | Placement policy affinity | Flags VMs with `migratable` placement policy affinity. Requires live migration support and RWX storage in the target. |
| `scsi_reservation.rego` | `ovirt.disk.scsi_reservation.enabled` | Shared disk (SCSI reservation) | Detects disks with SCSI reservation. Shared disks are unsupported in OpenShift Virtualization. |
| `secure_boot.rego` | `ovirt.secure_boot.detected` | UEFI secure boot detected | Detects UEFI secure boot (`q35_secure_boot` BIOS type). Only partially supported in OpenShift Virtualization. |
| `shared_disk.rego` | `ovirt.disk.shared.detected` | Shared disk detected | Flags disks marked as shared. Shared disks are unsupported in OpenShift Virtualization. |
| `tpm.rego` | `ovirt.tpm.required_by_os` | TPM detected | Detects Windows 2022 or Windows 11 VMs that require a TPM device. TPM data is not transferred during migration. |
| `usb.rego` | `ovirt.usb.enabled` | USB support enabled | Detects USB support being enabled. USB device attachment is unsupported in the target. |
| `vm_os.rego` | `ovirt.os.unsupported` | Unsupported operating system | Flags RHEL 6 (`rhel_6` / `rhel_6x64`) as an unsupported guest OS. |
| `watchdog.rego` | `ovirt.watchdog.enabled` | Watchdog detected | Detects watchdog devices. Not preserved during migration. |

### Information

| Rego File | Concern ID | Label | Description |
|-----------|-----------|-------|-------------|
| `ballooned_memory.rego` | `ovirt.memory.ballooning.enabled` | Memory ballooning enabled | Detects VMs with memory ballooning. Not supported in OpenShift Virtualization. |
| `display_type.rego` | `ovirt.display_type.spice.enabled` | VM Display Type (SPICE) | Detects VMs using the SPICE display protocol. SPICE is unsupported in OpenShift Virtualization. |
| `io_threads.rego` | `ovirt.iothreads.configured` | IO Threads configuration | Detects I/O thread configuration (more than 1 thread). Must be manually re-applied after migration. |
| `storage_error_resume_behaviour.rego` | `ovirt.storage.resume_behavior.unsupported` | Storage error resume behavior | Flags VMs where storage error resume behavior is not set to `auto_resume`. This setting is unsupported in the target. |

---

## OpenStack Provider

Policy directory: `validation/policies/io/konveyor/forklift/openstack/`

### Critical

| Rego File | Concern ID | Label | Description |
|-----------|-----------|-------|-------------|
| `disk_size.rego` | `openstack.disk.capacity.invalid` | Volume has an invalid size | Flags volumes with zero or negative size (in GB). |
| `disk_status.rego` | `openstack.disk.status.unsupported` | Unsupported disk status | Validates volume status is `available` or `in-use`. Other statuses may cause transfer failure. |
| `vm_status.rego` | `openstack.vm.status.invalid` | Invalid VM status | Validates VM status is `ACTIVE` or `SHUTOFF`. Other states may cause migration failure. |

### Warning

| Rego File | Concern ID | Label | Description |
|-----------|-----------|-------|-------------|
| `bios_boot_menu.rego` | `openstack.bios.boot_menu.enabled` | BIOS boot menu enabled | Detects BIOS boot menu via image property (`hw_boot_menu`). Unsupported in target. |
| `cpu_shares.rego` | `openstack.cpu.shares.defined` | CPU Shares Defined | Flags VMs with CPU shares in flavor extra specs (`quota:cpu_shares`). Not available in target. |
| `disk_interface_type.rego` | `openstack.disk.unsupported_interface` | Unsupported disk interface type | Validates disk bus type from image property (`hw_disk_bus`). Only `sata`, `scsi`, and `virtio` are supported. |
| `floating_ips.rego` | `openstack.network.floating_ips.detected` | Floating IPs detected | Detects floating IP assignments. This networking feature is not preserved in the target environment. |
| `host_devices.rego` | `openstack.host_devices.mapped` | VM has mapped host devices | Detects PCI passthrough via flavor extra specs (`pci_passthrough:alias`). Devices not attached post-migration. |
| `name.rego` | `openstack.vm.name.invalid` | Invalid VM Name | Validates VM name against RFC 1123. Non-compliant names are auto-renamed during migration. |
| `numa_tune.rego` | `openstack.numa_tuning.detected` | NUMA tuning detected | Detects NUMA configuration via flavor extra specs (`hw:numa_nodes`, `hw:pci_numa_affinity_policy`). Not preserved. |
| `secure_boot.rego` | `openstack.secure_boot.detected` | UEFI secure boot detected | Detects secure boot requirement via image property or flavor extra spec. Only partially supported. |
| `shared_disk.rego` | `openstack.disk.shared.detected` | Shared disk detected | Flags volumes attached to multiple instances. Shared disks are unsupported. |
| `vif_models.rego` | `openstack.network.vif_model.unsupported` | Unsupported VIF model | Validates VIF model from image property (`hw_vif_model`). Supported: `e1000`, `e1000e`, `rtl8139`, `virtio`, `ne2k_pci`, `pcnet`. |
| `vm_os.rego` | `openstack.os.unsupported` | Unsupported operating system | Validates guest OS via image properties (`os_distro` + `os_version`). Supported: RHEL/CentOS 7–9, Windows, Fedora 36–38. |
| `watchdog.rego` | `openstack.watchdog.detected` | Watchdog detected | Detects watchdog configuration via flavor or image properties. Not preserved during migration. |

---

## OVA Provider

Policy directory: `validation/policies/io/konveyor/forklift/ova/`

### Critical

| Rego File | Concern ID | Label | Description |
|-----------|-----------|-------|-------------|
| `disk_size.rego` | `ova.disk.capacity.invalid` | Disk has an invalid capacity | Flags disks with zero or negative capacity (in bytes). |

### Warning

| Rego File | Concern ID | Label | Description |
|-----------|-----------|-------|-------------|
| `cpu_affinity.rego` | `ova.cpu_affinity.detected` | CPU affinity detected | Detects CPU affinity. The VM migrates without it; administrators can reconfigure post-migration. |
| `cpu_memory_hotplug.rego` | `ova.cpu_memory.hotplug.enabled` | CPU/Memory hotplug detected | Detects CPU hot-add, CPU hot-remove, or memory hot-add. Unsupported in OpenShift Virtualization. |
| `export_source.rego` | `ova.source.unsupported` | Unsupported OVA source | Warns if the OVA was not exported from VMware. Non-VMware OVAs may encounter import issues. |
| `name.rego` | `ova.name.invalid` | Invalid VM Name | Validates VM name against RFC 1123. Non-compliant names are auto-renamed during migration. |

---

## Notes

- **Storage Array Type**: There is no existing validation that checks the underlying storage array hardware, vendor, or protocol (e.g. FibreChannel SAN, iSCSI, NFS, vSAN). The closest policy is oVirt's `disk_storage_type.rego`, which validates the disk storage type field (`image` or `lun`).
- **Cross-file dependencies**: oVirt's `disk_storage_type.rego` references `number_of_disks`, which is defined in `disk_interface_type.rego`. Both files must be loaded together for the policy to evaluate correctly.
- **Severity levels**: Concerns with category `Critical` are migration blockers. `Warning` concerns indicate lost functionality. `Information` concerns are purely advisory.
