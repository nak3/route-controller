/*
Copyright 2020 The Knative Authors

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

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	"time"

	scheme "github.com/nak3/gateway-api/pkg/client/gatewayapi/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
	v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// GatewayClassesGetter has a method to return a GatewayClassInterface.
// A group's client should implement this interface.
type GatewayClassesGetter interface {
	GatewayClasses() GatewayClassInterface
}

// GatewayClassInterface has methods to work with GatewayClass resources.
type GatewayClassInterface interface {
	Create(ctx context.Context, gatewayClass *v1alpha1.GatewayClass, opts v1.CreateOptions) (*v1alpha1.GatewayClass, error)
	Update(ctx context.Context, gatewayClass *v1alpha1.GatewayClass, opts v1.UpdateOptions) (*v1alpha1.GatewayClass, error)
	UpdateStatus(ctx context.Context, gatewayClass *v1alpha1.GatewayClass, opts v1.UpdateOptions) (*v1alpha1.GatewayClass, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.GatewayClass, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.GatewayClassList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.GatewayClass, err error)
	GatewayClassExpansion
}

// gatewayClasses implements GatewayClassInterface
type gatewayClasses struct {
	client rest.Interface
}

// newGatewayClasses returns a GatewayClasses
func newGatewayClasses(c *NetworkingV1alpha1Client) *gatewayClasses {
	return &gatewayClasses{
		client: c.RESTClient(),
	}
}

// Get takes name of the gatewayClass, and returns the corresponding gatewayClass object, and an error if there is any.
func (c *gatewayClasses) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.GatewayClass, err error) {
	result = &v1alpha1.GatewayClass{}
	err = c.client.Get().
		Resource("gatewayclasses").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of GatewayClasses that match those selectors.
func (c *gatewayClasses) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.GatewayClassList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.GatewayClassList{}
	err = c.client.Get().
		Resource("gatewayclasses").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested gatewayClasses.
func (c *gatewayClasses) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Resource("gatewayclasses").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a gatewayClass and creates it.  Returns the server's representation of the gatewayClass, and an error, if there is any.
func (c *gatewayClasses) Create(ctx context.Context, gatewayClass *v1alpha1.GatewayClass, opts v1.CreateOptions) (result *v1alpha1.GatewayClass, err error) {
	result = &v1alpha1.GatewayClass{}
	err = c.client.Post().
		Resource("gatewayclasses").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(gatewayClass).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a gatewayClass and updates it. Returns the server's representation of the gatewayClass, and an error, if there is any.
func (c *gatewayClasses) Update(ctx context.Context, gatewayClass *v1alpha1.GatewayClass, opts v1.UpdateOptions) (result *v1alpha1.GatewayClass, err error) {
	result = &v1alpha1.GatewayClass{}
	err = c.client.Put().
		Resource("gatewayclasses").
		Name(gatewayClass.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(gatewayClass).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *gatewayClasses) UpdateStatus(ctx context.Context, gatewayClass *v1alpha1.GatewayClass, opts v1.UpdateOptions) (result *v1alpha1.GatewayClass, err error) {
	result = &v1alpha1.GatewayClass{}
	err = c.client.Put().
		Resource("gatewayclasses").
		Name(gatewayClass.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(gatewayClass).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the gatewayClass and deletes it. Returns an error if one occurs.
func (c *gatewayClasses) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Resource("gatewayclasses").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *gatewayClasses) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Resource("gatewayclasses").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched gatewayClass.
func (c *gatewayClasses) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.GatewayClass, err error) {
	result = &v1alpha1.GatewayClass{}
	err = c.client.Patch(pt).
		Resource("gatewayclasses").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
