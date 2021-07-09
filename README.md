# Knative Gateway API Controller

[![GoDoc](https://godoc.org/knative.dev/gateway-api-controller?status.svg)](https://godoc.org/knative.dev/gateway-api-controller)
[![Go Report Card](https://goreportcard.com/badge/knative/gateway-api-controller)](https://goreportcard.com/report/knative/gateway-api-controller)

Knative Gateway API Controller is a controller to generate Gateway resources based on Knative Route.

## Usage

#### Deploy gateway-api CRD

```
kubectl apply -k 'github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.2.0'
```

#### Install Istio (v1.10 or later)

```
istioctl install -y
```

#### Install Knative Serving

```
kubectl apply --filename https://storage.googleapis.com/knative-nightly/serving/latest/serving-crds.yaml
kubectl apply --filename https://storage.googleapis.com/knative-nightly/serving/latest/serving-core.yaml
```

#### Install gateway-api controller

```
ko apply -f config/
```

#### Deploy Knative Service with `features.knative.dev/gateway-api` annotation

```
cat <<EOF | kubectl apply -f -
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: hello-example
  annotations:
    features.knative.dev/gateway-api: "enabled" 
spec:
  template:
    spec:
      containers:
      - image: gcr.io/knative-samples/helloworld-go
        name: user-container
EOF
```

Now httproute is created.

```
$ kubectl get httproutes.networking.x-k8s.io
NAME            HOSTNAMES
hello-example   ["hello-example.default","hello-example.default.svc","hello-example.default.svc.cluster.local","hello-example.default.example.com"]
```

#### Access to the knative service

```
$ curl -H "Host: hello-example.default.example.com" 172.20.0.2:30348
Hello World!
```

__NOTE__ `172.20.0.2:30348` needs to be replaced with your `istio-ingressgateway.istio-system` endpoint.

### (Known Issue) Knative Route status is "IngressNotConfigured"

Yes, serving code needs the following patch.

```diff
diff --git a/pkg/reconciler/route/route.go b/pkg/reconciler/route/route.go
index bccdede71..f22c09840 100644
--- a/pkg/reconciler/route/route.go
+++ b/pkg/reconciler/route/route.go
@@ -143,6 +143,15 @@ func (c *Reconciler) ReconcileKind(ctx context.Context, r *v1.Route) pkgreconcil
                        Host:   resourcenames.K8sServiceFullname(r),
                },
        }
+       const GatewayAPIAnnotation = "features.knative.dev/gateway-api"
+
+       if mode := r.GetAnnotations()["features.knative.dev/gateway-api"]; mode == "enabled" {
+               r.Status.Traffic, err = traffic.GetRevisionTrafficTargets(ctx, r, traffic.BuildRollout())
+               if err != nil {
+                       return err
+               }
+               return nil
+       }
 
        logger.Info("Creating placeholder k8s services")
        services, err := c.reconcilePlaceholderServices(ctx, r, traffic.Targets)
```

To learn more about Knative, please visit our
[Knative docs](https://github.com/knative/docs) repository.

If you are interested in contributing, see [CONTRIBUTING.md](./CONTRIBUTING.md)
and [DEVELOPMENT.md](./DEVELOPMENT.md).

