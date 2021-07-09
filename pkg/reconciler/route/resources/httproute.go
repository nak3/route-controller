package resources

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/pkg/kmeta"
	"knative.dev/serving/pkg/activator"
	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

// MakeHTTPRoute creates HTTPRoute to set up routing rules.
func MakeHTTPRoute(
	ctx context.Context,
	r *servingv1.Route,
	gateways []gwv1alpha1.Gateway,
) (*gwv1alpha1.HTTPRoute, error) {
	spec, err := makeHTTPRouteSpec(ctx, r, gateways)
	if err != nil {
		return nil, err
	}
	return &gwv1alpha1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
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
	gateways []gwv1alpha1.Gateway,
) (gwv1alpha1.HTTPRouteSpec, error) {

	domains := []gwv1alpha1.Hostname{}
	for _, traffic := range r.Status.Traffic {
		d, err := hostnames(ctx, traffic.Tag, r)
		if err != nil {
			return gwv1alpha1.HTTPRouteSpec{}, err
		}
		domains = append(domains, d...)
	}

	rules := []gwv1alpha1.HTTPRouteRule{makeHTTPRouteRule(r.Namespace, r.Status.Traffic)}

	gatewayRefs := []gwv1alpha1.GatewayReference{}
	for _, gw := range gateways {
		gatewayRefs = append(gatewayRefs, gwv1alpha1.GatewayReference{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		})
	}

	return gwv1alpha1.HTTPRouteSpec{
		Hostnames: domains,
		Rules:     rules,
		Gateways: gwv1alpha1.RouteGateways{
			Allow:       gwv1alpha1.GatewayAllowFromList,
			GatewayRefs: gatewayRefs,
		},
	}, nil
}

func makeHTTPRouteRule(ns string, targets []servingv1.TrafficTarget) gwv1alpha1.HTTPRouteRule {
	// Optimistically allocate |targets| elements.
	splits := make([]gwv1alpha1.HTTPRouteForwardTo, 0, len(targets))
	for _, t := range targets {
		if *t.Percent == 0 {
			continue
		}
		// TODO: protocol
		portNum := gwv1alpha1.PortNumber(networking.ServicePort(networking.ProtocolHTTP1))
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
							activator.RevisionHeaderName:      t.RevisionName,
							activator.RevisionHeaderNamespace: ns,
						}}},
				}})
	}

	return gwv1alpha1.HTTPRouteRule{
		ForwardTo: splits,
	}
}

func hostnames(ctx context.Context, targetName string, r *servingv1.Route) ([]gwv1alpha1.Hostname, error) {
	hostnames := []gwv1alpha1.Hostname{}

	visibilityToDomain, err := VisibilityToDomain(ctx, targetName, r)
	if err != nil {
		return hostnames, err
	}

	for _, v := range visibilityToDomain {
		hostnames = append(hostnames, v...)
	}

	return hostnames, err
}
