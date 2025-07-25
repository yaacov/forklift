package powerflex

import (
	"fmt"
	"slices"

	"github.com/dell/goscaleio"
	siotypes "github.com/dell/goscaleio/types/v1"
	"github.com/kubev2v/forklift/cmd/vsphere-xcopy-volume-populator/internal/populator"
	"k8s.io/klog/v2"
)

const sdcIDContextKey string = "sdcId"

type PowerflexClonner struct {
	Client *goscaleio.Client
}

// CurrentMappedGroups implements populator.StorageApi.
func (p *PowerflexClonner) CurrentMappedGroups(targetLUN populator.LUN, mappingContext populator.MappingContext) ([]string, error) {
	klog.Infof("getting current mapping to volume %+v", targetLUN)
	v, err := p.Client.GetVolume("", "", "", targetLUN.Name, false)
	if err != nil {
		return nil, err
	}
	currentMappedSdcs := []string{}
	if len(v) != 1 {
		return nil, fmt.Errorf("found %d volumes while expecting one. Target volume ID %s", len(v), targetLUN.ProviderID)
	}

	klog.Infof("current mapping %+v", v[0].MappedSdcInfo)
	for _, sdcInfo := range v[0].MappedSdcInfo {
		currentMappedSdcs = append(currentMappedSdcs, sdcInfo.SdcID)
	}
	return currentMappedSdcs, nil
}

// EnsureClonnerIgroup implements populator.StorageApi.
func (p *PowerflexClonner) EnsureClonnerIgroup(initiatorGroup string, clonnerIqn []string) (populator.MappingContext, error) {

	klog.Infof("ensuring initiator group %s for clonners %v", initiatorGroup, clonnerIqn)

	mappingContext := make(map[string]any)
	system, err := p.Client.FindSystem("", "", "")
	if err != nil {
		return nil, err
	}
	sdcs, err := system.GetSdc()
	if err != nil {
		return nil, err
	}

	for _, sdc := range sdcs {
		if sdc.OSType != "Esx" {
			continue
		}
		klog.Infof("@@@@@@@@@@@ sdc %+v", sdc)
		klog.Infof("@@@@@@@@@@@ clonnerIqn %+v", clonnerIqn)
		klog.Infof("@@@@@@@@@@@ sdc.SdcGUID %+v", sdc.SdcGUID)
		if slices.Contains(clonnerIqn, sdc.SdcGUID) {
			klog.Infof("found SDC ID %+v", sdc)
			mappingContext[sdcIDContextKey] = sdc.ID
			return mappingContext, nil
		}
	}

	// TODO it is possible that there is nothing to do here, only the map/unmap is needed
	return nil, nil
}

func (p *PowerflexClonner) Map(initiatorGroup string, targetLUN populator.LUN, mappingContext populator.MappingContext) (populator.LUN, error) {
	sdcId, ok := mappingContext[sdcIDContextKey]
	if ok {
		initiatorGroup = sdcId.(string)
	}
	klog.Infof("mapping volume %s to initiator group %s with context %v", targetLUN.Name, initiatorGroup, mappingContext)
	sdc, volume, err := p.fetchSdcVolume(initiatorGroup, targetLUN, mappingContext)

	if len(volume.Volume.MappedSdcInfo) > 0 {
		klog.Infof("unmapping the volume as it is mapped mapped already %+v", volume.Volume.MappedSdcInfo)
		unmapParams := siotypes.UnmapVolumeSdcParam{
			SdcID: volume.Volume.MappedSdcInfo[0].SdcID,
		}
		err := volume.UnmapVolumeSdc(&unmapParams)
		if err != nil {
			return targetLUN, fmt.Errorf("failed to unmap the volume from the former SDC %w", err)
		}
	}
	mapParams := siotypes.MapVolumeSdcParam{
		SdcID: sdc.Sdc.ID,
	}
	err = volume.MapVolumeSdc(&mapParams)
	if err != nil {
		return targetLUN, fmt.Errorf("failed to map the volume id %s to sdc id %s: %w", volume.Volume.ID, sdc.Sdc.ID, err)
	}
	// the serial or the NAA is the {$systemID$volumeID}
	targetLUN.NAA = fmt.Sprintf("eui.%s%s", sdc.Sdc.SystemID, volume.Volume.ID)
	return targetLUN, nil
}

// Map implements populator.StorageApi.
func (p *PowerflexClonner) fetchSdcVolume(initatorGroup string, targetLUN populator.LUN, mappingContext populator.MappingContext) (*goscaleio.Sdc, *goscaleio.Volume, error) {

	// TODO rgolan do we need an instanceID as part of the client?
	// probably yes for multiple instances
	system, err := p.Client.FindSystem("", "", "")
	if err != nil {
		return nil, nil, err
	}

	sdc, err := system.FindSdc("ID", initatorGroup)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to locate sdc by sdc guid %s", initatorGroup)
	}
	klog.Infof("found sdc name %s id %s", sdc.Sdc.Name, sdc.Sdc.ID)

	v, err := p.Client.GetVolume("", "", "", targetLUN.Name, false)
	if err != nil {
		return nil, nil, err
	}
	if len(v) != 1 {
		return nil, nil, fmt.Errorf("expected a single volume but found %d", len(v))
	}
	volumeService := goscaleio.NewVolume(p.Client)
	volumeService.Volume = v[0]
	return sdc, volumeService, nil
}

// ResolveVolumeHandleToLUN implements populator.StorageApi.
func (p *PowerflexClonner) ResolvePVToLUN(pv populator.PersistentVolume) (populator.LUN, error) {
	name := pv.VolumeAttributes["Name"]
	if name == "" {
		return populator.LUN{},
			fmt.Errorf("The PersistentVolume attribute 'Name' is empty and " +
				"essential to locate the underlying volume in PowerFlex")
	}
	id, err := p.Client.FindVolumeID(name)
	if err != nil {
		return populator.LUN{}, err

	}
	v, err := p.Client.GetVolume("", id, "", "", false)
	if err != nil {
		return populator.LUN{}, nil
	}

	if len(v) != 1 {
		return populator.LUN{}, fmt.Errorf("failed to locate a single volume by name %s.", name)
	}

	klog.Infof("found volume %s", v[0].Name)
	return populator.LUN{
		Name:         v[0].Name,
		ProviderID:   v[0].ID,
		VolumeHandle: pv.VolumeHandle,
	}, nil
}

// UnMap implements populator.StorageApi.
func (p *PowerflexClonner) UnMap(initatorGroup string, targetLUN populator.LUN, mappingContext populator.MappingContext) error {
	klog.Infof("unmapping volume %s from initiator group %s", targetLUN.Name, initatorGroup)
	sdc, volume, err := p.fetchSdcVolume(initatorGroup, targetLUN, mappingContext)
	if err != nil {
		return err
	}
	mapParams := siotypes.UnmapVolumeSdcParam{
		SdcID: sdc.Sdc.ID,
	}
	err = volume.UnmapVolumeSdc(&mapParams)
	if err != nil {
		return err
	}
	return nil
}

func NewPowerflexClonner(hostname, username, password string, sslSkipVerify bool) (PowerflexClonner, error) {
	client, err := goscaleio.NewClientWithArgs(hostname, "", 10000, sslSkipVerify, true)
	if err != nil {
		return PowerflexClonner{}, err
	}

	_, err = client.Authenticate(&goscaleio.ConfigConnect{
		Endpoint: hostname,
		Username: username,
		Password: password,
		Insecure: sslSkipVerify,
	})
	if err != nil {
		return PowerflexClonner{}, fmt.Errorf("error authenticating: %w", err)
	}

	klog.Infof("successfuly logged in to ScaleIO Gateway at %s version %s", client.GetConfigConnect().Endpoint, client.GetConfigConnect().Version)

	return PowerflexClonner{Client: client}, nil
}
