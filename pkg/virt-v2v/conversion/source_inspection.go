package conversion

import (
	"os"
)

// RunSourceV2vInspection inspects the vSphere source guest before conversion.
// virt-v2v-inspector does not accept guestfish-style -a disks from virt-v2v-open,
// so we connect directly with the same libvirt/VDDK input args as virt-v2v.
func (c *Conversion) RunSourceV2vInspection() error {
	v2vCmdBuilder := c.CommandBuilder.New("virt-v2v-inspector").
		AddFlag("-v").
		AddFlag("-x").
		AddArg("-O", c.InspectionOutputFile)

	if err := c.addVirtV2vVsphereArgsForInspection(v2vCmdBuilder); err != nil {
		return err
	}
	c.addInspectorExtraArgs(v2vCmdBuilder)

	v2vCmd := v2vCmdBuilder.Build()
	v2vCmd.SetStdout(os.Stdout)
	v2vCmd.SetStderr(os.Stderr)
	return v2vCmd.Run()
}
