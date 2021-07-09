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

package config

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"knative.dev/pkg/configmap"
	"knative.dev/serving/pkg/apis/serving"
)

const (
	// GatewayConfigName is the config map name for the gateway configuration.
	GatewayConfigName = "config-gateway"

	// DefaultGateway holds the kubernetes gateway that Route's live under by default
	// when no label selector-based options apply.
	// TODO: do not use istio's by default.
	DefaultGateway = "knative-example-gateway.istio-system"

	// DefaultLocalGateway holds the kubernetes local gateway that Route's live under by default
	DefaultLocalGateway = "knative-example-local-gateway.istio-system"

	// DefaultLocalService holds the default local gateway service address.
	// Placeholder service points to the service.
	DefaultLocalService = "knative-local-gateway.istio-system.svc.cluster.local"
)

type Address struct {
	Gateway string `json:"gateway,omitempty"`
	Service string `json:"service,omitempty"`
}

// Gateway maps gateways to routes by matching the gateway's
// label selectors to the route's labels.
type Gateway struct {
	// Gateways map from gateway to label selector.  If a route has
	// labels matching a particular selector, it will use the
	// corresponding gateway.  If multiple selectors match, we choose
	// the most specific selector.
	Gateways map[string]*Address
}

// NewGatewayFromConfigMap creates a Gateway from the supplied ConfigMap
func NewGatewayFromConfigMap(configMap *corev1.ConfigMap) (*Gateway, error) {
	c := Gateway{Gateways: map[string]*Address{}}
	for k, v := range configMap.Data {
		if k == configmap.ExampleKey {
			continue
		}
		address := &Address{}
		err := yaml.Unmarshal([]byte(v), address)
		if err != nil {
			return nil, err
		}
		c.Gateways[k] = address
	}
	// Add default gateway if empty key is not defined.
	if _, ok := c.Gateways[""]; !ok {
		c.Gateways[""] = &Address{Gateway: DefaultGateway}
	}

	// Add default local gateway if cluster-local gateway is not defined.
	if _, ok := c.Gateways[serving.VisibilityClusterLocal]; !ok {
		c.Gateways[serving.VisibilityClusterLocal] = &Address{DefaultLocalGateway, DefaultLocalService}
	}
	return &c, nil
}

// LookupGatewayForLabels returns a gateway given a set of labels.
// Since we reject configuration without a default gateway, this returns
// one or more gateways.
func (c *Gateway) LookupGatewayForLabels(visibility string) []string {
	gateways := sets.NewString()
	gateways.Insert(c.Gateways[visibility].Gateway)
	// Always add cluster-local gateway.
	gateways.Insert(c.Gateways[serving.VisibilityClusterLocal].Gateway)
	return gateways.UnsortedList()
}

// LookupServiceForLabels returns a gateway given a visibility config.
func (c *Gateway) LookupServiceForLabels(visibility string) string {
	return c.Gateways[visibility].Service
}
