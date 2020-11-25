package controllers

import (
	"context"
	"encoding/json"
	"github.com/ghodss/yaml"
	networkingv1alpha1 "github.com/softonic/rate-limit-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/softonic/rate-limit-operator/api/istio_v1alpha3"
)

func (r *RateLimitReconciler) applyEnvoyFilter(desired istio_v1alpha3.EnvoyFilter, found *istio_v1alpha3.EnvoyFilter, nameEnvoyFilter string, controllerNamespace string) error {

	err := r.Get(context.TODO(), types.NamespacedName{
		Namespace: controllerNamespace,
		Name:      nameEnvoyFilter,
	}, found)
	if err != nil {
		klog.Infof("Cannot Found EnvoyFilter %s before creating. Error %v", found.Name, err)
		err = r.Create(context.TODO(), &desired)
		if err != nil {
			klog.Infof("Cannot Create EnvoyFilter %s. Error %v", desired.Name, err)
			return err
		}
		klog.Infof("Creating %s...", desired.Name)
	} else {

		applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("rate-limit-controller")}

		err = r.Patch(context.TODO(), &desired, client.Apply, applyOpts...)
		if err != nil {
			klog.Infof("Cannot Patch EnvoyFilter %s. Error %v", desired.Name, err)
			return err
		}
		return nil
	}

	return nil

}

func (r *RateLimitReconciler) CreateOrUpdateConfigMap(rateLimitInstance *networkingv1alpha1.RateLimit, istioNamespace string, baseName string) error {

	var err error

	cm := r.generateConfigMap(rateLimitInstance, istioNamespace, baseName)
	if err != nil {
		klog.Infof("Cannot generate %v, Error: %v", cm, err)
		return err
	}

	found := v1.ConfigMap{}

	found, err = r.getConfigMap(baseName, istioNamespace)
	if err != nil {
		err = r.Create(context.TODO(), &cm)
		if err != nil {
			//return ctrl.Result{}, client.IgnoreNotFound(err)
			klog.Infof("Cannot create %v, Error: %v", cm, err)
		}
	} else if !reflect.DeepEqual(cm, found) {

		applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("rate-limit-controller")}

		err = r.Patch(context.TODO(), &cm, client.Apply, applyOpts...)
		if err != nil {
			return err
		}
	}

	return nil

}

func (r *RateLimitReconciler) generateConfigMap(rateLimitInstance *networkingv1alpha1.RateLimit, istioNamespace string, name string) v1.ConfigMap {

	configMapData := make(map[string]string)

	var output []byte

	descriptorOutput := networkingv1alpha1.OutputConfig{}

	descriptorOutput.DescriptorsParent = make([]networkingv1alpha1.DescriptorsParent, len(rateLimitInstance.Spec.Rate))

	descriptorOutput.Domain = name

	for k, dimension := range rateLimitInstance.Spec.Rate {
		descriptorOutput.DescriptorsParent[k].Key = dimension.Unit
		descriptor := networkingv1alpha1.Descriptors{
			Key: "destination_cluster",
			RateLimit: networkingv1alpha1.RateLimitPerDescriptor{
				RequestsPerUnit: dimension.RequestPerUnit,
				Unit:            dimension.Unit,
			},
			Value: rateLimitInstance.Spec.DestinationCluster,
		}
		descriptorOutput.DescriptorsParent[k].Descriptors = append(descriptorOutput.DescriptorsParent[k].Descriptors, descriptor)
	}

	output, _ = json.Marshal(descriptorOutput)

	y, _ := yaml.JSONToYAML(output)

	fileName := name + ".yaml"

	configMapData[fileName] = string(y)

	configMap := v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: istioNamespace,
		},
		Data: configMapData,
	}

	return configMap

}

func (r *RateLimitReconciler) UpdateDeployment(volumeProjectedSources []v1.VolumeProjection, volumes []v1.Volume) error {

	err := r.addVolumeFromDeployment(volumeProjectedSources, volumes)
	if err != nil {
		klog.Infof("Cannot add VolumeSource from deploy %v. Error %v", r.DeploymentRL, err)
		return err
	}

	err = r.Update(context.TODO(), &r.DeploymentRL)
	if err != nil {
		klog.Infof("Cannot Update Deployment %s. Error %v", "istio-system-ratelimit", err)
		return err
	}

	return nil

}

func (r *RateLimitReconciler) addVolumeFromDeployment(volumeProjectedSources []v1.VolumeProjection, volumes []v1.Volume) error {

	defaultVolumeMount := []v1.VolumeMount{
		{
			Name:      "commonconfig-volume",
			MountPath: "/data/ratelimit/config",
		},
	}

	//if len(r.DeploymentRL.Spec.Template.Spec.Volumes) == 0 {
	//	r.DeploymentRL.Spec.Template.Spec.Volumes = append(r.DeploymentRL.Spec.Template.Spec.Volumes, volumes...)
	//	r.DeploymentRL.Spec.Template.Spec.Containers[0].VolumeMounts = defaultVolumeMount
	//	return nil
	//}

	count := 0
	for _, v := range r.DeploymentRL.Spec.Template.Spec.Volumes {
		if v.Name == "commonconfig-volume" {
			v.VolumeSource.Projected.Sources = append(v.VolumeSource.Projected.Sources, volumeProjectedSources...)
		} else {
			count++
			//deploy.Spec.Template.Spec.Volumes = append(deploy.Spec.Template.Spec.Volumes, volumes...)
		}
	}

	if count > 0 {
		r.DeploymentRL.Spec.Template.Spec.Volumes = append(r.DeploymentRL.Spec.Template.Spec.Volumes, volumes...)
		r.DeploymentRL.Spec.Template.Spec.Containers[0].VolumeMounts = append(r.DeploymentRL.Spec.Template.Spec.Containers[0].VolumeMounts, defaultVolumeMount...)
	}

	return nil

}
