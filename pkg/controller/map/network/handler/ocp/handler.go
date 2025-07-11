package ocp

import (
	"context"
	"path"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/ocp"
	"github.com/kubev2v/forklift/pkg/controller/watch/handler"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	libweb "github.com/kubev2v/forklift/pkg/lib/inventory/web"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// Package logger.
var log = logging.WithName("networkMap|ocp")

// Provider watch event handler.
type Handler struct {
	*handler.Handler
}

// Ensure watch on networks.
// OCP inventory doesn't support watches. Instead, a generic event is sent to
// the channel periodically to trigger reconciliation.
func (r *Handler) Watch(watch *handler.WatchManager) (err error) {
	watch.EnsurePeriodicEvents(
		r.Provider(),
		&ocp.NetworkAttachmentDefinition{},
		handler.DefaultEventInterval,
		r.generateEvents)
	log.Info(
		"Periodic Inventory events ensured.",
		"provider",
		path.Join(
			r.Provider().Namespace,
			r.Provider().Name))

	return
}

// Resource created.
func (r *Handler) Created(e libweb.Event) {
	log.Info("OCP doesn't support web watches, this should not be called",
		"provider",
		path.Join(
			r.Provider().Namespace,
			r.Provider().Name))
}

// Resource deleted.
func (r *Handler) Deleted(e libweb.Event) {
	log.Info("OCP doesn't support web watches, this should not be called",
		"provider",
		path.Join(
			r.Provider().Namespace,
			r.Provider().Name))

}

// Send a generic event to the channel for all associated CRs.
func (r *Handler) generateEvents() {
	list := api.NetworkMapList{}
	err := r.List(context.TODO(), &list)
	if err != nil {
		err = liberr.Wrap(err)
		log.Error(err, "failed to list NetworkMap CRs")
		return
	}
	for i := range list.Items {
		mp := &list.Items[i]
		if r.MatchProvider(mp.Spec.Provider.Source) || r.MatchProvider(mp.Spec.Provider.Destination) {
			r.Enqueue(event.GenericEvent{
				Object: mp,
			})
		}
	}
}
