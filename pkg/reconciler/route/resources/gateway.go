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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	network "knative.dev/networking/pkg"
	ingress "knative.dev/networking/pkg/ingress"
	"knative.dev/pkg/kmeta"
	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/nak3/gateway-api/pkg/reconciler/route/config"
)

// GatewayAPIAnnotation uses the Gateway API resouces instead of KIngress with the value 'enabled'.
const GatewayAPIAnnotation = "features.knative.dev/gateway-api"

const (
	HTTPRouteLabelKey          = serving.GroupName + "/httproute"
	HTTPRouteNamespaceLabelKey = serving.GroupName + "/httprouteNamespace"
)

func VisibilityToDomain(ctx context.Context, targetName string, r *servingv1.Route) (map[string][]gwv1alpha1.Hostname, error) {
	visibilityToDomain := map[string][]gwv1alpha1.Hostname{}

	hostname, err := HostnameFromTemplate(ctx, r.Name, targetName)
	if err != nil {
		return map[string][]gwv1alpha1.Hostname{}, err
	}

	visibility := r.GetLabels()[network.VisibilityLabelKey]
	if visibility == "" {
		visibility = config.DefaultVisibilityName
	}

	meta := r.ObjectMeta.DeepCopy()

	// If cluster-local label is not added, add the external domain.
	domains := []gwv1alpha1.Hostname{}
	if visibility != serving.VisibilityClusterLocal {
		domain, err := DomainNameFromTemplate(ctx, *meta, hostname)
		if err != nil {
			return map[string][]gwv1alpha1.Hostname{}, err
		}
		domains = append(domains, gwv1alpha1.Hostname(domain))
	}

	visibilityToDomain[visibility] = domains

	// Always add local domain regardless visibility label.
	ldomains := []gwv1alpha1.Hostname{}
	meta.Labels[network.VisibilityLabelKey] = serving.VisibilityClusterLocal

	domain, err := DomainNameFromTemplate(ctx, *meta, hostname)
	if err != nil {
		return map[string][]gwv1alpha1.Hostname{}, err
	}
	expanded := ingress.ExpandedHosts(sets.NewString(domain)).List()
	for _, ex := range expanded {
		ldomains = append(ldomains, gwv1alpha1.Hostname(ex))
	}

	visibilityToDomain[serving.VisibilityClusterLocal] = ldomains

	return visibilityToDomain, err
}

// MakeGateway creates Gateway to map to HTTPRoute.
func MakeGateway(
	ctx context.Context,
	r *servingv1.Route,
	hosts []gwv1alpha1.Hostname,
	visibility string,
) (*gwv1alpha1.Gateway, error) {

	gatewayConfig := config.FromContext(ctx).Gateway
	gatewayNamespace := gatewayConfig.LookupGatewayNamespace(visibility)
	if gatewayNamespace == "" {
		gatewayNamespace = r.Namespace
	}

	return &gwv1alpha1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayName(r, visibility),
			Namespace: gatewayNamespace,
			Labels: kmeta.UnionMaps(r.Labels, map[string]string{
				serving.RouteLabelKey:          r.Name,
				serving.RouteNamespaceLabelKey: r.Namespace,
			}),
			Annotations: kmeta.FilterMap(r.GetAnnotations(), func(key string) bool {
				return key == corev1.LastAppliedConfigAnnotation
			}),
		},
		Spec: makeGatewaySpec(ctx, r, hosts, visibility, gatewayConfig),
	}, nil
}

// TODO: append unique suffix. It still duplicates with other gateway.
//
// GatewayName returns the name for the Gateway
// for the given Route and visibility.
func GatewayName(route kmeta.Accessor, visibility string) string {
	if visibility != "" {
		visibility = "-" + visibility
	}
	return kmeta.ChildName(route.GetName()+"-"+route.GetNamespace(), visibility)
}

func makeGatewaySpec(
	ctx context.Context,
	r *servingv1.Route,
	hosts []gwv1alpha1.Hostname,
	visibility string,
	gwConfig *config.Gateway,
) gwv1alpha1.GatewaySpec {

	var listeners []gwv1alpha1.Listener
	for _, host := range hosts {
		host := host
		route := gwv1alpha1.RouteBindingSelector{
			Namespaces: gwv1alpha1.RouteNamespaces{
				From: gwv1alpha1.RouteSelectAll,
			},
			Selector: metav1.LabelSelector{MatchLabels: map[string]string{
				serving.RouteLabelKey:          r.Name,
				serving.RouteNamespaceLabelKey: r.Namespace,
			}},
			Kind: "HTTPRoute",
		}
		listeners = append(listeners, gwv1alpha1.Listener{
			Hostname: &host,
			Port:     gwv1alpha1.PortNumber(80),
			Protocol: gwv1alpha1.HTTPProtocolType,
			Routes:   route})
	}

	gwSpec := gwv1alpha1.GatewaySpec{
		GatewayClassName: gwConfig.LookupGatewayClass(visibility),
		Listeners:        listeners,
	}
	if ad := gwConfig.LookupAddress(visibility); ad != "" {
		gwSpec.Addresses = []gwv1alpha1.GatewayAddress{{Type: gwv1alpha1.NamedAddressType, Value: ad}}
	}
	return gwSpec
}
