/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package route

import (
	"context"

	netclient "knative.dev/networking/pkg/client/injection/client"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	serviceinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/service"
	servingclient "knative.dev/serving/pkg/client/injection/client"
	routeinformer "knative.dev/serving/pkg/client/injection/informers/serving/v1/route"
	routereconciler "knative.dev/serving/pkg/client/injection/reconciler/serving/v1/route"

	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/tools/cache"
	network "knative.dev/networking/pkg"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/tracker"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/reconciler/route/config"

	gwapiclient "github.com/nak3/gateway-api/pkg/client/gatewayapi/injection/client"
	httprouteinformer "github.com/nak3/gateway-api/pkg/client/gatewayapi/injection/informers/apis/v1alpha1/httproute"
)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	return newController(ctx, cmw, clock.RealClock{})
}

type reconcilerOption func(*Reconciler)

func newController(
	ctx context.Context,
	cmw configmap.Watcher,
	clock clock.Clock,
	opts ...reconcilerOption,
) *controller.Impl {
	logger := logging.FromContext(ctx)
	serviceInformer := serviceinformer.Get(ctx)
	routeInformer := routeinformer.Get(ctx)
	httprouteInformer := httprouteinformer.Get(ctx)

	c := &Reconciler{
		kubeclient:      kubeclient.Get(ctx),
		client:          servingclient.Get(ctx),
		netclient:       netclient.Get(ctx),
		gwapiclient:     gwapiclient.Get(ctx),
		serviceLister:   serviceInformer.Lister(),
		httprouteLister: httprouteInformer.Lister(),
		clock:           clock,
	}
	impl := routereconciler.NewImpl(ctx, c, func(impl *controller.Impl) controller.Options {
		configsToResync := []interface{}{
			&network.Config{},
			&config.Domain{},
			&config.Gateway{},
		}
		resync := configmap.TypeFilter(configsToResync...)(func(string, interface{}) {
			impl.GlobalResync(routeInformer.Informer())
		})
		configStore := config.NewStore(logging.WithLogger(ctx, logger.Named("config-store")), resync)
		configStore.WatchConfigs(cmw)
		return controller.Options{ConfigStore: configStore}
	})
	c.enqueueAfter = impl.EnqueueAfter

	logger.Info("Setting up event handlers")
	routeInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	handleControllerOf := cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterController(&v1.Route{}),
		Handler:    controller.HandleAll(impl.EnqueueControllerOf),
	}
	serviceInformer.Informer().AddEventHandler(handleControllerOf)
	certificateInformer.Informer().AddEventHandler(handleControllerOf)
	httprouteInformer.Informer().AddEventHandler(handleControllerOf)

	c.tracker = tracker.New(impl.EnqueueKey, controller.GetTrackerLease(ctx))

	// Make sure trackers are deleted once the observers are removed.
	routeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.tracker.OnDeletedObserver,
	})

	for _, opt := range opts {
		opt(c)
	}
	return impl
}
