/*
Copyright 2022.

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

package controllers

import (
	"fmt"

	buildv1alpha1 "github.com/kairos-io/osbuilder/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *NetbootDeploymentReconciler) genDeployment(artifact buildv1alpha1.NetbootDeployment) *appsv1.Deployment {
	// TODO: svc is unused, but could be used in the future to generate the Netboot URL
	objMeta := metav1.ObjectMeta{
		Name:            artifact.Name,
		Namespace:       artifact.Namespace,
		OwnerReferences: genNetbootOwner(artifact),
	}

	privileged := false
	serviceAccount := false

	servingContainer := v1.Container{
		ImagePullPolicy: v1.PullAlways,
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
		Name:            "serve",
		Args: []string{

			"boot",
			fmt.Sprintf(),
			fmt.Sprintf(),
			fmt.Sprintf(),
		},
		Image: r.Image,
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/files/config.yaml",
				SubPath:   "cloudconfig",
			},
		},
	}
	// boot /files/kairos-core-opensuse-kernel /files/kairos-core-opensuse-initrd --cmdline='rd.neednet=1 ip=dhcp rd.cos.disable root=live:https://github.com/kairos-io/provider-kairos/releases/download/v1.2.0/kairos-alpine-ubuntu-v1.2.0-k3sv1.20.15+k3s1.squashfs netboot nodepair.enable config_url={{ ID "/files/config.yaml" }} console=tty1 console=ttyS0 console=tty0'
	pod := v1.PodSpec{
		HostNetwork:                  true,
		AutomountServiceAccountToken: &serviceAccount,
		Volumes: []v1.Volume{
			{
				Name: "config",
				VolumeSource: v1.VolumeSource{
					ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: v1.LocalObjectReference{Name: fmt.Sprintf("%s-netboot", artifact.Name)}}},
			},
		},
	}

	pod.Containers = []v1.Container{servingContainer}

	deploymentLabels := genNetDeploymentLabel(artifact.Name)
	replicas := int32(1)

	return &appsv1.Deployment{
		ObjectMeta: objMeta,

		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: deploymentLabels},
			Replicas: &replicas,
			Template: v1.PodTemplateSpec{

				ObjectMeta: metav1.ObjectMeta{
					Labels: deploymentLabels,
				},
				Spec: pod,
			},
		},
	}
}
func genNetDeploymentLabel(s string) map[string]string {
	return map[string]string{
		"netboot": "serve" + s,
	}
}
