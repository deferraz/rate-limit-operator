package controllers

import (
	"context"
	"encoding/json"
	"github.com/softonic/rate-limit-operator/api/istio_v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/softonic/rate-limit-operator/api/istio_v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
)

func (r *RateLimitReconciler) getK8sResources(baseName string, istioNamespace string, controllerNamespace string, deploymentName string) error {

	r.getEnvoyFilters(baseName, istioNamespace)

	var err error

	r.configMapRateLimit, err = r.getConfigMap(baseName, controllerNamespace)
	if err != nil {
		klog.Infof("Cannot Found ConfigMap in the getk8sresource func %s. Error %v", baseName, err)

	}

	return nil
}

func getConfigObjectMatch(typeConfigObjectMatch string, operation string, clusterEndpoint string, context string, nameVhost string, route string) istio_v1alpha3.EnvoyConfigObjectMatch {

	Match := istio_v1alpha3.EnvoyConfigObjectMatch{}

	vhost := istio_v1alpha3.RouteConfigurationMatch_VirtualHostMatch{}

	if typeConfigObjectMatch == "Listener" {

		Match = istio_v1alpha3.EnvoyConfigObjectMatch{
			Context: context,
			Listener: &istio_v1alpha3.ListenerMatch{
				FilterChain: istio_v1alpha3.ListenerMatch_FilterChainMatch{
					Filter: istio_v1alpha3.ListenerMatch_FilterMatch{
						Name: "envoy.filters.network.http_connection_manager",
						SubFilter: istio_v1alpha3.ListenerMatch_SubFilterMatch{
							Name: "envoy.filters.http.router",
						},
					},
				},
			},
		}

	}

	if typeConfigObjectMatch == "Cluster" {

		Match = istio_v1alpha3.EnvoyConfigObjectMatch{
			Cluster: &istio_v1alpha3.ClusterMatch{
				Service: clusterEndpoint,
			},
		}

	}

	if route != "" {
		vhost = istio_v1alpha3.RouteConfigurationMatch_VirtualHostMatch{
			Route: istio_v1alpha3.RouteConfigurationMatch_RouteMatch{
				Action: "ANY",
				Name:   route,
			},
		}
	} else {
		vhost = istio_v1alpha3.RouteConfigurationMatch_VirtualHostMatch{
			Name: nameVhost,
			Route: istio_v1alpha3.RouteConfigurationMatch_RouteMatch{
				Action: "ANY",
			},
		}
	}

	if typeConfigObjectMatch == "RouteConfiguration" {

		Match = istio_v1alpha3.EnvoyConfigObjectMatch{
			Context: context,
			RouteConfiguration: &istio_v1alpha3.RouteConfigurationMatch{
				Vhost: vhost,
			},
		}

	}

	return Match

}

func getEnvoyFilterConfigPatches(applyTo string, operation string, rawConfig json.RawMessage, typeConfigObjectMatch string, clusterEndpoint string, context string, nameVhost string, routes []string) []istio_v1alpha3.EnvoyConfigObjectPatch {

	ConfigPatches := []istio_v1alpha3.EnvoyConfigObjectPatch{}
	element := istio_v1alpha3.EnvoyConfigObjectPatch{}

	if len(routes) > 0 {
		for _, route := range routes {
			element = istio_v1alpha3.EnvoyConfigObjectPatch{
				ApplyTo: applyTo,
				Patch: istio_v1alpha3.Patch{
					Operation: operation,
					Value:     rawConfig,
				},
				Match: getConfigObjectMatch(typeConfigObjectMatch, operation, clusterEndpoint, context, nameVhost, route),
			}
			ConfigPatches = append(ConfigPatches, element)
		}
	} else {
		ConfigPatches = []istio_v1alpha3.EnvoyConfigObjectPatch{
			{
				ApplyTo: applyTo,
				Patch: istio_v1alpha3.Patch{
					Operation: operation,
					Value:     rawConfig,
				},
				Match: getConfigObjectMatch(typeConfigObjectMatch, operation, clusterEndpoint, context, nameVhost, ""),
			},
		}
	}

	return ConfigPatches

}

func (e EnvoyFilterObject) composeEnvoyFilter(name string, namespace string) istio_v1alpha3.EnvoyFilter {

	envoyFilterBaseDesired := istio_v1alpha3.EnvoyFilter{
		TypeMeta: metav1.TypeMeta{
			Kind:       "EnvoyFilter",
			APIVersion: "networking.istio.io/v1alpha3",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: istio_v1alpha3.EnvoyFilterSpec{
			WorkloadSelector: istio_v1alpha3.WorkloadSelector{
				Labels: e.Labels,
			},
			ConfigPatches: getEnvoyFilterConfigPatches(e.ApplyTo, e.Operation, e.RawConfig, e.TypeConfigObjectMatch, e.ClusterEndpoint, e.Context, e.NameVhost, e.Routes),
		},
	}

	return envoyFilterBaseDesired

}

func (r *RateLimitReconciler) getEnvoyFilters(baseName string, istioNamespace string) *[]*istio_v1alpha3.EnvoyFilter {

	// case switch with the type of the filter

	envoyFilterCluster := r.getEnvoyFilter(baseName+"-cluster", istioNamespace)

	envoyFilterHTTPFilter := r.getEnvoyFilter(baseName+"-envoy-filter", istioNamespace)

	envoyFilterHTTPRoute := r.getEnvoyFilter(baseName+"-route", istioNamespace)

	r.EnvoyFilters = append(r.EnvoyFilters, envoyFilterCluster, envoyFilterHTTPFilter, envoyFilterHTTPRoute)

	return &r.EnvoyFilters

}

func (r *RateLimitReconciler) getEnvoyFilter(name string, namespace string) *istio_v1alpha3.EnvoyFilter {

	envoyFilter := istio_v1alpha3.EnvoyFilter{}

	err := r.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, &envoyFilter)
	if err != nil {
		klog.Infof("Cannot Found EnvoyFilter %s. Error %v", name, err)
		return &envoyFilter
	}

	return &envoyFilter

}

func (r *RateLimitReconciler) getConfigMap(name string, namespace string) (v1.ConfigMap, error) {

	found := v1.ConfigMap{}

	err := r.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, &found)
	if err != nil {
		//klog.Infof("Cannot Found configMap %s. Error %v", found.Name, err)
		return found, err
	}

	return found, nil

}

func constructVolumeSources(name string) []v1.VolumeProjection {

	//sources := make([]v1.VolumeProjection, 0)

	sources := []v1.VolumeProjection{
		{
			ConfigMap: &v1.ConfigMapProjection{
				LocalObjectReference: v1.LocalObjectReference{
					Name: name,
				},
			},
		},
	}

	return sources
}

func constructVolumes(nameVolume string, nameVolumeSource string) []v1.Volume {

	var defaultMode int32

	defaultMode = 0420

	p := &defaultMode

	sources := constructVolumeSources(nameVolumeSource)

	//	Volumes := make([]v1.Volume, 0)

	Volumes := []v1.Volume{
		{
			Name: nameVolume,
			VolumeSource: v1.VolumeSource{
				Projected: &v1.ProjectedVolumeSource{
					DefaultMode: p,
					Sources:     sources,
				},
			},
		},
	}

	return Volumes
}

func (r *RateLimitReconciler) getDeployment(controllerNamespace string, name string) (appsv1.Deployment, error) {

	found := appsv1.Deployment{}

	//klog.Infof("Before getting this deployment")

	deploy := &appsv1.Deployment{}
	err := r.Get(context.TODO(), client.ObjectKey{
		Namespace: controllerNamespace,
		Name:      name,
	}, deploy)
	if err != nil {
		klog.Infof("Cannot Get Deployment %s. Error %v", "istio-system-ratelimit", err)
		return found, err
	}

	//klog.Infof("Getting this deployment %v", deploy)

	return *deploy, nil
}

func (r *RateLimitReconciler) getVirtualService(namespace string, name string) (*istio_v1beta1.VirtualService, error) {

	virtualService := &istio_v1beta1.VirtualService{}
	err := r.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, virtualService)
	if err != nil {
		return nil, err
	}

	return virtualService, nil

}

func (r *RateLimitReconciler) getGateway(namespace string, name string) (*istio_v1beta1.Gateway, error) {

	Gateway := &istio_v1beta1.Gateway{}
	err := r.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, Gateway)
	if err != nil {
		return nil, err
	}

	return Gateway, nil

}
