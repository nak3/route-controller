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
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubelabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	network "knative.dev/networking/pkg"
	"knative.dev/networking/pkg/apis/networking"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	netclientset "knative.dev/networking/pkg/client/clientset/versioned"
	networkinglisters "knative.dev/networking/pkg/client/listers/networking/v1alpha1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
	"knative.dev/serving/pkg/apis/serving"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	clientset "knative.dev/serving/pkg/client/clientset/versioned"
	routereconciler "knative.dev/serving/pkg/client/injection/reconciler/serving/v1/route"
	listers "knative.dev/serving/pkg/client/listers/serving/v1"
	kaccessor "knative.dev/serving/pkg/reconciler/accessor"
	networkaccessor "knative.dev/serving/pkg/reconciler/accessor/networking"
	"knative.dev/serving/pkg/reconciler/route/config"
	"knative.dev/serving/pkg/reconciler/route/domains"
	"knative.dev/serving/pkg/reconciler/route/resources"
	"knative.dev/serving/pkg/reconciler/route/resources/labels"
	resourcenames "knative.dev/serving/pkg/reconciler/route/resources/names"
	"knative.dev/serving/pkg/reconciler/route/traffic"
	"knative.dev/serving/pkg/reconciler/route/visibility"

	gwapiclientset "github.com/nak3/gateway-api/pkg/client/gatewayapi/clientset/versioned"
	gwlisters "github.com/nak3/gateway-api/pkg/client/gatewayapi/listers/apis/v1alpha1"
)

// Reconciler implements controller.Reconciler for Route resources.
type Reconciler struct {
	kubeclient  kubernetes.Interface
	client      clientset.Interface
	netclient   netclientset.Interface
	gwapiclient gwapiclientset.Interface

	// Listers index properties about resources
	configurationLister listers.ConfigurationLister
	revisionLister      listers.RevisionLister
	serviceLister       corev1listers.ServiceLister
	ingressLister       networkinglisters.IngressLister
	httprouteLister     gwlisters.HTTPRouteLister
	certificateLister   networkinglisters.CertificateLister
	tracker             tracker.Interface

	clock        clock.PassiveClock
	enqueueAfter func(interface{}, time.Duration)
}

// Check that our Reconciler implements routereconciler.Interface
var _ routereconciler.Interface = (*Reconciler)(nil)

func ingressClassForRoute(ctx context.Context, r *v1.Route) string {
	if ingressClass := r.Annotations[networking.IngressClassAnnotationKey]; ingressClass != "" {
		return ingressClass
	}
	return config.FromContext(ctx).Network.DefaultIngressClass
}

func certClass(ctx context.Context, r *v1.Route) string {
	if class := r.Annotations[networking.CertificateClassAnnotationKey]; class != "" {
		return class
	}
	return config.FromContext(ctx).Network.DefaultCertificateClass
}

// getPlaceholderServiceNames returns the placeholder services names, or an error.
func (c *Reconciler) getPlaceholderServiceNames(route *v1.Route) (sets.String, error) {
	currentServices, err := c.serviceLister.Services(route.Namespace).List(
		kubelabels.SelectorFromSet(kubelabels.Set{serving.RouteLabelKey: route.Name}),
	)
	if err != nil {
		return nil, err
	}

	names := make(sets.String, len(currentServices))
	for _, svc := range currentServices {
		names.Insert(svc.Name)
	}

	return names, nil
}

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, r *v1.Route) pkgreconciler.Event {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	logger := logging.FromContext(ctx)
	logger.Debugf("Reconciling route: %#v", r.Spec)

	// When a new generation is observed for the first time, we need to make sure that we
	// do not report ourselves as being ready prematurely due to an error during
	// reconciliation.  For instance, if we were to hit an error creating new placeholder
	// service, we might report "Ready: True" with a bumped ObservedGeneration without
	// having updated the kingress at all!
	// We hit this in: https://github.com/knative-sandbox/net-contour/issues/238
	if r.GetObjectMeta().GetGeneration() != r.Status.ObservedGeneration {
		r.Status.MarkIngressNotConfigured()
	}

	// Configure traffic based on the RouteSpec.
	traffic, err := c.configureTraffic(ctx, r)
	if traffic == nil || err != nil {
		if err != nil {
			r.Status.MarkUnknownTrafficError(err.Error())
		}
		// Traffic targets aren't ready, no need to configure child resources.
		return err
	}

	r.Status.Address = &duckv1.Addressable{
		URL: &apis.URL{
			Scheme: "http",
			Host:   resourcenames.K8sServiceFullname(r),
		},
	}

	logger.Info("Creating placeholder k8s services")
	services, err := c.reconcilePlaceholderServices(ctx, r, traffic.Targets)
	if err != nil {
		return err
	}

	if mode := r.GetAnnotations()[resources.GatewayAPIAnnotation]; mode == "enabled" {
		httproute, err := c.reconcileHTTPRoute(ctx, r, traffic)
		if err != nil {
			return err
		}
		ready, err := IsHTTPRouteReady(httproute)
		if err != nil {
			return err
		}

		logger.Info("Updating placeholder k8s services with httproute information")
		if err := c.updatePlaceholderServices(ctx, r, services, nil); err != nil {
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
		} else {
			r.Status.MarkIngressNotConfigured()
		}
		return nil
	}

	logger.Info("Route successfully synced")
	return nil
}

// configureTraffic attempts to configure traffic based on the RouteSpec.  If there are missing
// targets (e.g. Configurations without a Ready Revision, or Revision that isn't Ready or Inactive),
// no traffic will be configured.
//
// If traffic is configured we update the RouteStatus with AllTrafficAssigned = True.  Otherwise we
// mark AllTrafficAssigned = False, with a message referring to one of the missing target.
func (c *Reconciler) configureTraffic(ctx context.Context, r *v1.Route) (*traffic.Config, error) {
	logger := logging.FromContext(ctx)
	t, trafficErr := traffic.BuildTrafficConfiguration(c.configurationLister, c.revisionLister, r)
	if t == nil {
		return nil, trafficErr
	}
	// Augment traffic configuration with visibility information.  Do not overwrite trafficErr,
	// since we will use it later.
	visibility, err := visibility.NewResolver(c.serviceLister).GetVisibility(ctx, r)
	if err != nil {
		return nil, err
	}
	t.Visibility = visibility
	// Update the Route URL.
	if err := c.updateRouteStatusURL(ctx, r, t.Visibility); err != nil {
		return nil, err
	}
	// Tell our trackers to reconcile Route whenever the things referred to by our
	// traffic stanza change. We also track missing targets since there may be
	// race conditions were routes are reconciled before their targets appear
	// in the informer cache
	for _, obj := range t.MissingTargets {
		if err := c.tracker.TrackReference(tracker.Reference{
			APIVersion: obj.APIVersion,
			Kind:       obj.Kind,
			Namespace:  obj.Namespace,
			Name:       obj.Name,
		}, r); err != nil {
			return nil, err
		}
	}
	for _, configuration := range t.Configurations {
		if err := c.tracker.TrackReference(objectRef(configuration), r); err != nil {
			return nil, err
		}
	}
	for _, revision := range t.Revisions {
		if revision.Status.IsActivationRequired() {
			logger.Infof("Revision %s/%s is inactive", revision.Namespace, revision.Name)
		}
		if err := c.tracker.TrackReference(objectRef(revision), r); err != nil {
			return nil, err
		}
	}

	var badTarget traffic.TargetError
	isTargetError := errors.As(trafficErr, &badTarget)
	if trafficErr != nil && !isTargetError {
		// An error that's not due to missing traffic target should
		// make us fail fast.
		return nil, trafficErr
	}
	if badTarget != nil && isTargetError {
		logger.Info("Marking bad traffic target: ", badTarget)
		badTarget.MarkBadTrafficTarget(&r.Status)

		// Traffic targets aren't ready, no need to configure Route.
		return nil, nil
	}

	logger.Info("All referred targets are routable, marking AllTrafficAssigned with traffic information.")

	// Pass empty rollout here. We'll recompute this if there is a rollout in progress.
	r.Status.Traffic, err = t.GetRevisionTrafficTargets(ctx, r, &traffic.Rollout{})
	if err != nil {
		return nil, err
	}

	r.Status.MarkTrafficAssigned()

	return t, nil
}

func (c *Reconciler) updateRouteStatusURL(ctx context.Context, route *v1.Route, visibility map[string]netv1alpha1.IngressVisibility) error {
	isClusterLocal := visibility[traffic.DefaultTarget] == netv1alpha1.IngressVisibilityClusterLocal

	mainRouteMeta := route.ObjectMeta.DeepCopy()
	labels.SetVisibility(mainRouteMeta, isClusterLocal)

	host, err := domains.DomainNameFromTemplate(ctx, *mainRouteMeta, route.Name)
	if err != nil {
		return err
	}

	scheme := "http"
	if !isClusterLocal {
		scheme = config.FromContext(ctx).Network.DefaultExternalScheme
	}
	route.Status.URL = &apis.URL{
		Scheme: scheme,
		Host:   host,
	}

	return nil
}

// GetNetworkingClient returns the client to access networking resources.
func (c *Reconciler) GetNetworkingClient() netclientset.Interface {
	return c.netclient
}

// GetCertificateLister returns the lister for Knative Certificate.
func (c *Reconciler) GetCertificateLister() networkinglisters.CertificateLister {
	return c.certificateLister
}

/////////////////////////////////////////
// Misc helpers.
/////////////////////////////////////////

type accessor interface {
	GetGroupVersionKind() schema.GroupVersionKind
	GetNamespace() string
	GetName() string
}

func objectRef(a accessor) tracker.Reference {
	gvk := a.GetGroupVersionKind()
	apiVersion, kind := gvk.ToAPIVersionAndKind()
	return tracker.Reference{
		APIVersion: apiVersion,
		Kind:       kind,
		Namespace:  a.GetNamespace(),
		Name:       a.GetName(),
	}
}

func getTrafficNames(targets map[string]traffic.RevisionTargets) []string {
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	return names
}

// Sets the traffic URL scheme to scheme if the URL matches the dnsNames.
// dnsNames are DNS names under a certificate for a particular domain, and so only change
// the corresponding traffic under the route, rather than all traffic
func setTargetsScheme(rs *v1.RouteStatus, dnsNames []string, scheme string) {
	for i := range rs.Traffic {
		if rs.Traffic[i].URL == nil {
			continue
		}
		for _, dnsName := range dnsNames {
			if rs.Traffic[i].URL.Host == dnsName {
				rs.Traffic[i].URL.Scheme = scheme
				break
			}
		}
	}
}

func autoTLSEnabled(ctx context.Context, r *v1.Route) bool {
	if !config.FromContext(ctx).Network.AutoTLS {
		return false
	}

	logger := logging.FromContext(ctx)
	annotationValue := r.Annotations[networking.DisableAutoTLSAnnotationKey]

	disabledByAnnotation, err := strconv.ParseBool(annotationValue)
	if annotationValue != "" && err != nil {
		// validation should've caught an invalid value here.
		// if we have one anyways, assume not disabled and log a warning.
		logger.Warnf("Invalid annotation value for %q. Value: %q",
			networking.DisableAutoTLSAnnotationKey, annotationValue)
	}

	return !disabledByAnnotation
}

func findMatchingWildcardCert(ctx context.Context, domains []string, certs []*netv1alpha1.Certificate) *netv1alpha1.Certificate {
	for _, cert := range certs {
		if wildcardCertMatches(ctx, domains, cert) {
			return cert
		}
	}
	return nil
}

func wildcardCertMatches(ctx context.Context, domains []string, cert *netv1alpha1.Certificate) bool {
	dnsNames := make(sets.String, len(cert.Spec.DNSNames))
	logger := logging.FromContext(ctx)

	for _, dns := range cert.Spec.DNSNames {
		dnsParts := strings.SplitAfterN(dns, ".", 2)
		if len(dnsParts) < 2 {
			logger.Infof("got non-FQDN DNSName %s in certificate %s", dns, cert.Name)
			continue
		}
		dnsNames.Insert(dnsParts[1])
	}
	for _, domain := range domains {
		domainParts := strings.SplitAfterN(domain, ".", 2)
		if len(domainParts) < 2 || !dnsNames.Has(domainParts[1]) {
			return false
		}
	}

	return true
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
