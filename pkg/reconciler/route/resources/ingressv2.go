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

package resources

import (
	"context"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	network "knative.dev/networking/pkg"
	"knative.dev/networking/pkg/apis/networking"
	ingress "knative.dev/networking/pkg/ingress"
	"knative.dev/pkg/kmeta"
	"knative.dev/serving/pkg/activator"
	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/reconciler/route/config"
	"knative.dev/serving/pkg/reconciler/route/domains"
	"knative.dev/serving/pkg/reconciler/route/resources/names"
	"knative.dev/serving/pkg/reconciler/route/traffic"

	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// GatewayAPIAnnotation uses the Gateway API resouces instead of KIngress with the value 'enabled'.
const GatewayAPIAnnotation = "features.knative.dev/gateway-api"

// MakeHTTPRoute creates HTTPRoute to set up routing rules.
func MakeHTTPRoute(
	ctx context.Context,
	r *servingv1.Route,
	tc *traffic.Config) (*gwv1alpha1.HTTPRoute, error) {
	spec, err := makeHTTPRouteSpec(ctx, r, tc)
	if err != nil {
		return nil, err
	}
	return &gwv1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Ingress(r),
			Namespace: r.Namespace,
			Labels: kmeta.UnionMaps(r.Labels, map[string]string{
				serving.RouteLabelKey:          r.Name,
				serving.RouteNamespaceLabelKey: r.Namespace,
			}),
			Annotations: kmeta.FilterMap(r.GetAnnotations(), func(key string) bool {
				return key == corev1.LastAppliedConfigAnnotation
			}),
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(r)},
		},
		Spec: spec,
	}, nil
}

func makeHTTPRouteSpec(
	ctx context.Context,
	r *servingv1.Route,
	tc *traffic.Config,
) (gwv1alpha1.HTTPRouteSpec, error) {
	// Domain should have been specified in route status
	// before calling this func.
	names := make([]string, 0, len(tc.Targets))
	for name := range tc.Targets {
		names = append(names, name)
	}
	// Sort the names to give things a deterministic ordering.
	sort.Strings(names)
	// The routes are matching rule based on domain name to traffic split targets.
	rules := make([]gwv1alpha1.HTTPRouteRule, 0, len(names))

	domains := []gwv1alpha1.Hostname{}
	for _, name := range names {
		var err error
		domains, err = gwDomain(ctx, name, r)
		if err != nil {
			return gwv1alpha1.HTTPRouteSpec{}, err
		}
		rule := makeHTTPRouteRule(r.Namespace, tc.Targets[name])

		rules = append(rules, rule)
	}

	gateways := lookupGateway(ctx, r)

	return gwv1alpha1.HTTPRouteSpec{
		Gateways:  gateways,
		Hostnames: domains,
		Rules:     rules,
	}, nil
}

func lookupGateway(ctx context.Context, r *servingv1.Route) gwv1alpha1.RouteGateways {
	gatewayRefs := []gwv1alpha1.GatewayReference{}

	gatewayConfig := config.FromContext(ctx).Gateway
	gateways := gatewayConfig.LookupGatewayForLabels(r.GetLabels()[network.VisibilityLabelKey])
	for _, gateway := range gateways {
		parts := strings.Split(gateway, ".")
		if len(parts) < 2 {
			// TODO: validation?
		}
		name, namespace := parts[0], parts[1]
		gatewayRefs = append(gatewayRefs,
			gwv1alpha1.GatewayReference{
				Namespace: namespace,
				Name:      name,
			})
	}

	return gwv1alpha1.RouteGateways{
		Allow:       gwv1alpha1.GatewayAllowFromList,
		GatewayRefs: gatewayRefs,
	}
}

func gwDomain(ctx context.Context, targetName string, r *servingv1.Route) ([]gwv1alpha1.Hostname, error) {

	gwHostname := []gwv1alpha1.Hostname{}

	hostname, err := domains.HostnameFromTemplate(ctx, r.Name, targetName)
	if err != nil {
		return []gwv1alpha1.Hostname{}, err
	}

	meta := r.ObjectMeta.DeepCopy()
	// If external Gateway, add the external domain.
	if meta.GetLabels()[network.VisibilityLabelKey] != serving.VisibilityClusterLocal {
		domain, err := domains.DomainNameFromTemplate(ctx, *meta, hostname)
		if err != nil {
			return []gwv1alpha1.Hostname{}, err
		}
		gwHostname = append(gwHostname, gwv1alpha1.Hostname(domain))
	}

	// Always add local domain.
	meta.Labels[network.VisibilityLabelKey] = serving.VisibilityClusterLocal
	domain, err := domains.DomainNameFromTemplate(ctx, *meta, hostname)
	if err != nil {
		return []gwv1alpha1.Hostname{}, err
	}
	domains := ingress.ExpandedHosts(sets.NewString(domain)).List()
	for _, d := range domains {
		gwHostname = append(gwHostname, gwv1alpha1.Hostname(d))
	}

	return gwHostname, err
}

func makeHTTPRouteRule(ns string,
	targets traffic.RevisionTargets) gwv1alpha1.HTTPRouteRule {
	return *makeBaseHTTPRoutePath(ns, targets)
}

func makeBaseHTTPRoutePath(ns string, targets traffic.RevisionTargets) *gwv1alpha1.HTTPRouteRule {
	// Optimistically allocate |targets| elements.
	splits := make([]gwv1alpha1.HTTPRouteForwardTo, 0, len(targets))
	for _, t := range targets {
		if *t.Percent == 0 {
			continue
		}
		portNum := gwv1alpha1.PortNumber(networking.ServicePort(t.Protocol))
		splits = append(splits,
			gwv1alpha1.HTTPRouteForwardTo{
				ServiceName: &t.RevisionName,
				// Port on the public service must match port on the activator.
				// Otherwise, the serverless services can't guarantee seamless positive handoff.
				Port:   &portNum,
				Weight: int32(*t.Percent),
				Filters: []gwv1alpha1.HTTPRouteFilter{{
					Type: gwv1alpha1.HTTPRouteFilterRequestHeaderModifier,
					RequestHeaderModifier: &gwv1alpha1.HTTPRequestHeaderFilter{
						Set: map[string]string{
							activator.RevisionHeaderName:      t.TrafficTarget.RevisionName,
							activator.RevisionHeaderNamespace: ns,
						}}},
				}})
	}

	return &gwv1alpha1.HTTPRouteRule{
		ForwardTo: splits,
	}
}
