/*
Copyright 2018 The Knative Authors

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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
	"knative.dev/serving/pkg/apis/serving"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	routereconciler "knative.dev/serving/pkg/client/injection/reconciler/serving/v1/route"

	gwapiclientset "github.com/nak3/gateway-api/pkg/client/gatewayapi/clientset/versioned"
	gwlisters "github.com/nak3/gateway-api/pkg/client/gatewayapi/listers/apis/v1alpha1"
	"github.com/nak3/gateway-api/pkg/reconciler/route/resources"
)

// Reconciler implements controller.Reconciler for Route resources.
type Reconciler struct {
	kubeclient  kubernetes.Interface
	gwapiclient gwapiclientset.Interface

	tracker      tracker.Interface
	clock        clock.PassiveClock
	enqueueAfter func(interface{}, time.Duration)

	// Listers index properties about resources
	serviceLister   corev1listers.ServiceLister
	httprouteLister gwlisters.HTTPRouteLister
	gatewayLister   gwlisters.GatewayLister
}

// Check that our Reconciler implements routereconciler.Interface
var _ routereconciler.Interface = (*Reconciler)(nil)

// FinalizeKind finalizes ingress resource.
func (c *Reconciler) FinalizeKind(ctx context.Context, r *v1.Route) pkgreconciler.Event {
	gws, err := c.gatewayLister.List(labels.SelectorFromSet(map[string]string{
		serving.RouteNamespaceLabelKey: r.GetNamespace(),
		serving.RouteLabelKey:          r.GetName(),
	}))
	if err != nil {
		return err
	}

	for _, gw := range gws {
		err := c.gwapiclient.NetworkingV1alpha1().Gateways(gw.Namespace).Delete(ctx, gw.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, r *v1.Route) pkgreconciler.Event {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	logger := logging.FromContext(ctx)
	logger.Debugf("Reconciling route: %#v", r.Spec)

	if mode := r.GetAnnotations()[resources.GatewayAPIAnnotation]; mode == "enabled" {

		logger.Info("Creating placeholder k8s services")
		err := c.reconcilePlaceholderServices(ctx, r)
		if err != nil {
			return err
		}

		gateways, err := c.reconcileGateway(ctx, r)
		if err != nil {
			return err
		}
		logger.Infof("Gateway successfully synced %v", gateways)

		httproute, err := c.reconcileHTTPRoute(ctx, r, gateways)
		if err != nil {
			return err
		}
		ready, err := IsHTTPRouteReady(httproute)
		if err != nil {
			return err
		}

		// TODO: Add new status field in Route for httproute instead of ingress.
		if ready {
			r.Status.PropagateIngressStatus(netv1alpha1.IngressStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{{
						Type:   netv1alpha1.IngressConditionReady,
						Status: corev1.ConditionTrue,
					}},
				},
			})
			r.Status.MarkTLSNotEnabled(v1.AutoTLSNotEnabledMessage)
		} else {
			r.Status.MarkIngressNotConfigured()
		}

	}

	logger.Info("Route successfully synced")
	return nil
}

// IsHTTPRouteReady will check the status conditions of the ingress and return true if
// all gateways have been admitted.
func IsHTTPRouteReady(r *gatewayv1alpha1.HTTPRoute) (bool, error) {
	if r.Status.Gateways == nil {
		return false, nil
	}
	for _, gw := range r.Status.Gateways {
		if !isGatewayAdmitted(gw) {
			// Return false if _any_ of the gateways isn't admitted yet.
			return false, nil
		}
	}
	return true, nil
}

func isGatewayAdmitted(gw gatewayv1alpha1.RouteGatewayStatus) bool {
	for _, condition := range gw.Conditions {
		if condition.Type == string(gatewayv1alpha1.ConditionRouteAdmitted) {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
}
