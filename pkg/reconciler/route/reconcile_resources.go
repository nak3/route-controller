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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	"knative.dev/pkg/controller"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/reconciler/route/resources/names"

	"github.com/nak3/gateway-api/pkg/reconciler/route/config"
	"github.com/nak3/gateway-api/pkg/reconciler/route/resources"
)

// reconcileHTTPRoute reconciles HTTPRoute.
func (c *Reconciler) reconcileHTTPRoute(
	ctx context.Context, r *v1.Route,
	gateways []gwv1alpha1.Gateway,
) (*gwv1alpha1.HTTPRoute, error) {
	recorder := controller.GetEventRecorder(ctx)

	httproute, err := c.httprouteLister.HTTPRoutes(r.Namespace).Get(names.Ingress(r))
	if apierrs.IsNotFound(err) {
		desired, err := resources.MakeHTTPRoute(ctx, r, gateways)
		if err != nil {
			return nil, err
		}
		httproute, err = c.gwapiclient.NetworkingV1alpha1().HTTPRoutes(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			recorder.Eventf(r, corev1.EventTypeWarning, "CreationFailed", "Failed to create HTTPRoute: %v", err)
			return nil, fmt.Errorf("failed to create HTTPRoute: %w", err)
		}

		recorder.Eventf(r, corev1.EventTypeNormal, "Created", "Created HTTPRoute %q", httproute.GetName())
		return httproute, nil
	} else if err != nil {
		return nil, err
	} else {
		desired, err := resources.MakeHTTPRoute(ctx, r, gateways)
		if err != nil {
			return nil, err
		}

		if !equality.Semantic.DeepEqual(httproute.Spec, desired.Spec) ||
			!equality.Semantic.DeepEqual(httproute.Annotations, desired.Annotations) ||
			!equality.Semantic.DeepEqual(httproute.Labels, desired.Labels) {

			// Don't modify the informers copy.
			origin := httproute.DeepCopy()
			origin.Spec = desired.Spec
			origin.Annotations = desired.Annotations
			origin.Labels = desired.Labels

			updated, err := c.gwapiclient.NetworkingV1alpha1().HTTPRoutes(origin.Namespace).Update(
				ctx, origin, metav1.UpdateOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to update HTTPRoute: %w", err)
			}
			return updated, nil
		}
	}

	return httproute, err
}

// reconcileGateway reconciles HTTPRoute.
func (c *Reconciler) reconcileGateway(
	ctx context.Context, r *v1.Route,
) ([]gwv1alpha1.Gateway, error) {
	recorder := controller.GetEventRecorder(ctx)

	gws := []gwv1alpha1.Gateway{}

	visibilityToDomain := map[string][]gwv1alpha1.Hostname{}
	for _, traffic := range r.Status.Traffic {
		mp, err := resources.VisibilityToDomain(ctx, traffic.Tag, r)
		if err != nil {
			return nil, err
		}
		visibilityToDomain = UnionMaps(visibilityToDomain, mp)
	}

	gatewayConfig := config.FromContext(ctx).Gateway

	for visibility, hostnames := range visibilityToDomain {
		ns := gatewayConfig.LookupGatewayNamespace(visibility)
		if ns == "" {
			ns = r.Namespace
		}

		gateway, err := c.gatewayLister.Gateways(ns).Get(resources.GatewayName(r, visibility))
		if apierrs.IsNotFound(err) {
			desired, err := resources.MakeGateway(ctx, r, hostnames, visibility)
			if err != nil {
				return nil, err
			}
			gateway, err = c.gwapiclient.NetworkingV1alpha1().Gateways(ns).Create(ctx, desired, metav1.CreateOptions{})
			if err != nil {
				recorder.Eventf(r, corev1.EventTypeWarning, "CreationFailed", "Failed to create Gateway: %v", err)
				return nil, fmt.Errorf("failed to create Gateway: %w", err)
			}

			recorder.Eventf(r, corev1.EventTypeNormal, "Created", "Created Gateway %q", gateway.GetName())
			gws = append(gws, *gateway)
		} else if err != nil {
			return nil, err
		} else {
			// TODO: namespace change
			desired, err := resources.MakeGateway(ctx, r, hostnames, visibility)
			if err != nil {
				return nil, err
			}

			if !equality.Semantic.DeepEqual(gateway.Spec, desired.Spec) ||
				!equality.Semantic.DeepEqual(gateway.Annotations, desired.Annotations) ||
				!equality.Semantic.DeepEqual(gateway.Labels, desired.Labels) {

				// Don't modify the informers copy.
				origin := gateway.DeepCopy()
				origin.Spec = desired.Spec
				origin.Annotations = desired.Annotations
				origin.Labels = desired.Labels

				updated, err := c.gwapiclient.NetworkingV1alpha1().Gateways(origin.Namespace).Update(
					ctx, origin, metav1.UpdateOptions{})
				if err != nil {
					return nil, fmt.Errorf("failed to update HTTPRoute: %w", err)
				}
				gws = append(gws, *updated)
			}
		}
	}
	return gws, nil
}

// reconcilePlaceholderServices reconciles placholder k8s service.
func (c *Reconciler) reconcilePlaceholderServices(
	ctx context.Context, r *v1.Route,
) error {
	recorder := controller.GetEventRecorder(ctx)

	var targets []string
	for _, traffic := range r.Status.Traffic {
		target, err := resources.HostnameFromTemplate(ctx, r.Name, traffic.Tag)
		if err != nil {
			return err
		}
		targets = append(targets, target)
	}

	for _, target := range targets {
		svc, err := c.serviceLister.Services(r.Namespace).Get(target)
		if apierrs.IsNotFound(err) {
			desired, err := resources.MakePlaceholderService(ctx, r, target)
			if err != nil {
				return err
			}
			svc, err = c.kubeclient.CoreV1().Services(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
			if err != nil {
				recorder.Eventf(r, corev1.EventTypeWarning, "CreationFailed", "Failed to create Service: %v", err)
				return fmt.Errorf("failed to create Service: %w", err)
			}
			recorder.Eventf(r, corev1.EventTypeNormal, "Created", "Created HTTPRoute %q", svc.GetName())
			return nil
		} else if err != nil {
			return err
		} else {
			desired, err := resources.MakePlaceholderService(ctx, r, target)
			if err != nil {
				return err
			}

			if !equality.Semantic.DeepEqual(svc.Spec, desired.Spec) ||
				!equality.Semantic.DeepEqual(svc.Annotations, desired.Annotations) ||
				!equality.Semantic.DeepEqual(svc.Labels, desired.Labels) {

				// Don't modify the informers copy.
				origin := svc.DeepCopy()
				origin.Spec = desired.Spec
				origin.Annotations = desired.Annotations
				origin.Labels = desired.Labels

				_, err := c.kubeclient.CoreV1().Services(origin.Namespace).Update(
					ctx, origin, metav1.UpdateOptions{})
				if err != nil {
					return fmt.Errorf("failed to update Service: %w", err)
				}
				return nil
			}
		}
	}
	return nil
}

// UnionMaps returns a map constructed from the union of input maps.
// where values from latter maps win.
func UnionMaps(maps ...map[string][]gwv1alpha1.Hostname) map[string][]gwv1alpha1.Hostname {
	if len(maps) == 0 {
		return map[string][]gwv1alpha1.Hostname{}
	}
	out := make(map[string][]gwv1alpha1.Hostname, len(maps[0]))

	for _, m := range maps {
		for k, v := range m {
			out[k] = append(out[k], v...)
		}
	}
	return out
}
