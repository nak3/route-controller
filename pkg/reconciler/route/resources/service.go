package resources

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/pkg/kmeta"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/nak3/gateway-api/pkg/reconciler/route/config"
)

// MakePlaceholderService creates Kubernetes Service to map local service to
// local gateway service.
func MakePlaceholderService(
	ctx context.Context,
	r *servingv1.Route,
	hostname string,
) (*corev1.Service, error) {

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostname,
			Namespace: r.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				// This service is owned by the Route.
				*kmeta.NewControllerRef(r),
			},
			Labels:      r.GetLabels(),
			Annotations: r.GetAnnotations(),
		},
		Spec: makeServiceSpec(ctx),
	}, nil
}

func makeServiceSpec(ctx context.Context) corev1.ServiceSpec {
	gatewayConfig := config.FromContext(ctx).Gateway
	externalName := gatewayConfig.LookupAddress("cluster-local")

	return corev1.ServiceSpec{
		Type:            corev1.ServiceTypeExternalName,
		ExternalName:    externalName,
		SessionAffinity: corev1.ServiceAffinityNone,
		Ports: []corev1.ServicePort{{
			Name:       networking.ServicePortNameH2C,
			Port:       int32(80),
			TargetPort: intstr.FromInt(80),
		}},
	}

}
