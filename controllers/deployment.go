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

	buildv1alpha1 "github.com/c3os-io/osbuilder-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func genDeploymentLabel(s string) map[string]string {
	return map[string]string{
		"osbuild": "workload" + s,
	}
}
func (r *OSArtifactReconciler) genDeployment(artifact buildv1alpha1.OSArtifact) *appsv1.Deployment {
	objMeta := metav1.ObjectMeta{
		Name:            artifact.Name,
		Namespace:       artifact.Namespace,
		OwnerReferences: genOwner(artifact),
	}

	privileged := false
	serviceAccount := false
	buildIsoContainer := v1.Container{
		ImagePullPolicy: v1.PullAlways,
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
		Name:            "build-iso",
		Image:           r.BuildImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			fmt.Sprintf(
				"elemental --debug --name %s build-iso --date=false --overlay-iso /iso/iso-overlay %s --output /public",
				artifact.Name,
				artifact.Spec.ImageName,
			),
		},
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "public",
				MountPath: "/public",
			},
			{
				Name:      "config",
				MountPath: "/iso/iso-overlay/cloud_config.yaml",
				SubPath:   "config",
			},
			{
				Name:      "grub",
				MountPath: "/iso/iso-overlay/boot/grub2/grub.cfg",
				SubPath:   "grub.cfg",
			},
		},
	}

	if artifact.Spec.PullFromKube {
		buildIsoContainer.Args = []string{
			fmt.Sprintf(
				"elemental --debug --name %s build-iso --date=false --overlay-iso /iso/iso-overlay --output /public /rootfs",
				artifact.Name,
			),
		}
	}

	pullContainer := v1.Container{
		ImagePullPolicy: v1.PullAlways,
		Name:            "build-iso",
		Image:           artifact.Spec.ImageName,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			fmt.Sprintf(
				"rsync -aqAX --exclude='mnt' --exclude='proc' --exclude='sys' --exclude='dev' --exclude='tmp' %s %s",
				"/",
				"/rootfs",
			),
		},
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "rootfs",
				MountPath: "/rootfs",
			},
		},
	}

	servingContainer := v1.Container{
		ImagePullPolicy: v1.PullAlways,
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
		Name:            "serve",
		Ports:           []v1.ContainerPort{v1.ContainerPort{Name: "http", ContainerPort: 80}},
		Image:           r.ServingImage,
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "public",
				MountPath: "/usr/share/nginx/html",
			},
		},
	}

	pod := v1.PodSpec{
		AutomountServiceAccountToken: &serviceAccount,
		Volumes: []v1.Volume{
			{
				Name:         "public",
				VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
			},
			{
				Name:         "rootfs",
				VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
			},
			{
				Name: "config",
				VolumeSource: v1.VolumeSource{
					ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: v1.LocalObjectReference{Name: artifact.Name}}},
			},
		},
	}

	pod.InitContainers = []v1.Container{buildIsoContainer}
	if artifact.Spec.PullFromKube {
		// pull first
		pod.InitContainers = append([]v1.Container{pullContainer}, pod.InitContainers...)
	}
	pod.Containers = []v1.Container{servingContainer}

	deploymentLabels := genDeploymentLabel(artifact.Name)
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
