package customize

import (
	"fmt"

	"github.com/kubev2v/forklift/pkg/virt-v2v/utils"
)

// Prepare extracts embedded scripts and templates into the workdir.
func (c *Customize) Prepare() error {
	return c.embeddedFileSystem.CreateFilesFromFS(c.appConfig.Workdir)
}

// AppendConversionArgs adds virt-customize-compatible flags to a virt-v2v command.
func (c *Customize) AppendConversionArgs(cmd utils.CommandBuilder) error {
	if err := c.Prepare(); err != nil {
		return fmt.Errorf("failed to create files from filesystem: %w", err)
	}
	if c.operatingSystem.IsWindows() {
		return c.appendWindowsCustomizeArgs(cmd)
	}
	return c.appendLinuxCustomizeArgs(cmd)
}
