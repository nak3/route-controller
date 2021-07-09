package resources

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	network "knative.dev/networking/pkg"
	"knative.dev/serving/pkg/apis/serving"

	"github.com/nak3/gateway-api/pkg/reconciler/route/config"
)

//####################################################
//# These functions are copied from serving          #
//####################################################

// HostnameFromTemplate generates domain name base on the template specified in the `config-network` ConfigMap.
// name is the "subdomain" which will be referred as the "name" in the template
func HostnameFromTemplate(ctx context.Context, name, tag string) (string, error) {
	if tag == "" {
		return name, nil
	}
	// These are the available properties they can choose from.
	// We could add more over time - e.g. RevisionName if we thought that
	// might be of interest to people.
	data := network.TagTemplateValues{
		Name: name,
		Tag:  tag,
	}

	networkConfig := config.FromContext(ctx).Network
	buf := bytes.Buffer{}
	if err := networkConfig.GetTagTemplate().Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing the TagTemplate: %w", err)
	}
	return buf.String(), nil
}

// DomainNameFromTemplate generates domain name base on the template specified in the `config-network` ConfigMap.
// name is the "subdomain" which will be referred as the "name" in the template
func DomainNameFromTemplate(ctx context.Context, r metav1.ObjectMeta, name string) (string, error) {
	domainConfig := config.FromContext(ctx).Domain
	rLabels := r.Labels
	domain := domainConfig.LookupDomainForLabels(rLabels)
	annotations := r.Annotations
	// These are the available properties they can choose from.
	// We could add more over time - e.g. RevisionName if we thought that
	// might be of interest to people.
	data := network.DomainTemplateValues{
		Name:        name,
		Namespace:   r.Namespace,
		Domain:      domain,
		Annotations: annotations,
		Labels:      rLabels,
	}

	networkConfig := config.FromContext(ctx).Network
	buf := bytes.Buffer{}

	var templ *template.Template
	// If the route is "cluster local" then don't use the user-defined
	// domain template, use the default one
	if rLabels[network.VisibilityLabelKey] == serving.VisibilityClusterLocal {
		templ = template.Must(template.New("domain-template").Parse(
			network.DefaultDomainTemplate))
	} else {
		templ = networkConfig.GetDomainTemplate()
	}

	if err := templ.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing the DomainTemplate: %w", err)
	}

	urlErrs := validation.IsFullyQualifiedDomainName(field.NewPath("url"), buf.String())
	if urlErrs != nil {
		return "", fmt.Errorf("invalid domain name %q: %w", buf.String(), urlErrs.ToAggregate())
	}

	return buf.String(), nil
}
