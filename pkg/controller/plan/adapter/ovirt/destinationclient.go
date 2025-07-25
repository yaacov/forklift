package ovirt

import (
	"context"
	"path"

	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	core "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type DestinationClient struct {
	*plancontext.Context
}

// Delete OvirtVolumePopulator CustomResource list.
func (r *DestinationClient) DeletePopulatorDataSource(vm *plan.VMStatus) error {
	populatorCrList, err := r.getPopulatorCrList()
	if err != nil {
		return liberr.Wrap(err)
	}
	for _, populatorCr := range populatorCrList.Items {
		err = r.DeleteObject(&populatorCr, vm, "Deleted OvirtPopulator CR.", "OvirtVolumePopulator")
		if err != nil {
			return liberr.Wrap(err)
		}
	}
	return nil
}

// Set the OvirtVolumePopulator CustomResource Ownership.
func (r *DestinationClient) SetPopulatorCrOwnership() (err error) {
	populatorCrList, err := r.getPopulatorCrList()
	if err != nil {
		return
	}

	for _, populatorCr := range populatorCrList.Items {
		pvc, err := r.findPVCByCR(&populatorCr)
		if err != nil {
			continue
		}

		populatorCrCopy := populatorCr.DeepCopy()
		err = k8sutil.SetOwnerReference(pvc, &populatorCr, r.Scheme())
		if err != nil {
			continue
		}
		patch := client.MergeFrom(populatorCrCopy)
		err = r.Destination.Client.Patch(context.TODO(), &populatorCr, patch)
		if err != nil {
			continue
		}
	}
	return
}

// Get the OvirtVolumePopulator CustomResource List.
func (r *DestinationClient) getPopulatorCrList() (populatorCrList v1beta1.OvirtVolumePopulatorList, err error) {
	populatorCrList = v1beta1.OvirtVolumePopulatorList{}
	err = r.Destination.Client.List(
		context.TODO(),
		&populatorCrList,
		&client.ListOptions{
			Namespace:     r.Plan.Spec.TargetNamespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{"migration": string(r.Plan.Status.Migration.ActiveSnapshot().Migration.UID)}),
		})
	return
}

// Deletes an object from destination cluster associated with the VM.
func (r *DestinationClient) DeleteObject(object client.Object, vm *plan.VMStatus, message, objType string) (err error) {
	err = r.Destination.Client.Delete(context.TODO(), object)
	if err != nil {
		if k8serr.IsNotFound(err) {
			err = nil
		} else {
			return liberr.Wrap(err)
		}
	} else {
		r.Log.Info(
			message,
			objType,
			path.Join(
				object.GetNamespace(),
				object.GetName()),
			"vm",
			vm.String())
	}
	return
}

func (r *DestinationClient) findPVCByCR(cr *v1beta1.OvirtVolumePopulator) (pvc *core.PersistentVolumeClaim, err error) {
	pvcList := core.PersistentVolumeClaimList{}
	err = r.Destination.Client.List(
		context.TODO(),
		&pvcList,
		&client.ListOptions{
			Namespace: r.Plan.Spec.TargetNamespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"migration": string(r.Plan.Status.Migration.ActiveSnapshot().Migration.UID),
				"diskID":    cr.Spec.DiskID,
			}),
		})
	if err != nil {
		err = liberr.Wrap(err)
		return
	}

	if len(pvcList.Items) == 0 {
		err = liberr.New("PVC not found", "diskID", cr.Spec.DiskID)
		return
	}

	if len(pvcList.Items) > 1 {
		err = liberr.New("Multiple PVCs found", "diskID", cr.Spec.DiskID)
		return
	}

	pvc = &pvcList.Items[0]

	return
}
