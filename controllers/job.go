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

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	osbuilder "github.com/kairos-io/osbuilder/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
)

func unpackContainer(id, containerImage, pullImage string) corev1.Container {
	return corev1.Container{
		ImagePullPolicy: corev1.PullAlways,
		Name:            fmt.Sprintf("pull-image-%s", id),
		Image:           containerImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			fmt.Sprintf(
				"luet util unpack %s %s",
				pullImage,
				"/rootfs",
			),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "rootfs",
				MountPath: "/rootfs",
			},
		},
	}
}

func pushImageName(artifact *osbuilder.OSArtifact) string {
	pushName := artifact.Spec.ImageName
	if pushName != "" {
		return pushName
	}
	return artifact.Name
}

func createImageContainer(containerImage string, artifact *osbuilder.OSArtifact) corev1.Container {
	imageName := pushImageName(artifact)

	return corev1.Container{
		ImagePullPolicy: corev1.PullAlways,
		Name:            "create-image",
		Image:           containerImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			fmt.Sprintf(
				"tar -czvpf test.tar -C /rootfs . && luet util pack %[1]s test.tar %[2]s.tar && chmod +r %[2]s.tar && mv %[2]s.tar /artifacts",
				imageName,
				artifact.Name,
			),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "rootfs",
				MountPath: "/rootfs",
			},
			{
				Name:      "artifacts",
				MountPath: "/artifacts",
			},
		},
	}
}

func osReleaseContainer(containerImage string) corev1.Container {
	return corev1.Container{
		ImagePullPolicy: corev1.PullAlways,
		Name:            "os-release",
		Image:           containerImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			"cp -rfv /etc/os-release /rootfs/etc/os-release",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/etc/os-release",
				SubPath:   "os-release",
			},
			{
				Name:      "rootfs",
				MountPath: "/rootfs",
			},
		},
	}
}

func (r *OSArtifactReconciler) newArtifactPVC(artifact *osbuilder.OSArtifact) *corev1.PersistentVolumeClaim {
	if artifact.Spec.Volume == nil {
		artifact.Spec.Volume = &corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					"storage": resource.MustParse("10Gi"),
				},
			},
		}
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      artifact.Name + "-artifacts",
			Namespace: artifact.Namespace,
		},
		Spec: *artifact.Spec.Volume,
	}

	return pvc
}

func (r *OSArtifactReconciler) newBuilderPod(pvcName string, artifact *osbuilder.OSArtifact) *corev1.Pod {
	cmd := fmt.Sprintf(
		"/entrypoint.sh --debug --name %s build-iso --date=false --output /artifacts dir:/rootfs",
		artifact.Name,
	)

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "artifacts",
			MountPath: "/artifacts",
		},
		{
			Name:      "rootfs",
			MountPath: "/rootfs",
		},
	}

	if artifact.Spec.GRUBConfig != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "config",
			MountPath: "/iso/iso-overlay/boot/grub2/grub.cfg",
			SubPath:   "grub.cfg",
		})
	}

	cloudImgCmd := fmt.Sprintf(
		"/raw-images.sh /rootfs /artifacts/%s.raw",
		artifact.Name,
	)

	if artifact.Spec.CloudConfigRef != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "cloudconfig",
			MountPath: "/iso/iso-overlay/cloud_config.yaml",
			SubPath:   artifact.Spec.CloudConfigRef.Key,
		})

		cloudImgCmd += " /iso/iso-overlay/cloud_config.yaml"
	}

	if artifact.Spec.CloudConfigRef != nil || artifact.Spec.GRUBConfig != "" {
		cmd = fmt.Sprintf(
			"/entrypoint.sh --debug --name %s build-iso --date=false --overlay-iso /iso/iso-overlay --output /artifacts dir:/rootfs",
			artifact.Name,
		)
	}

	buildIsoContainer := corev1.Container{
		ImagePullPolicy: corev1.PullAlways,
		SecurityContext: &corev1.SecurityContext{Privileged: ptr(true)},
		Name:            "build-iso",
		Image:           r.ToolImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			cmd,
		},
		VolumeMounts: volumeMounts,
	}

	buildCloudImageContainer := corev1.Container{
		ImagePullPolicy: corev1.PullAlways,
		SecurityContext: &corev1.SecurityContext{Privileged: ptr(true)},
		Name:            "build-cloud-image",
		Image:           r.ToolImage,

		Command: []string{"/bin/bash", "-cxe"},
		Args: []string{
			cloudImgCmd,
		},
		VolumeMounts: volumeMounts,
	}

	if artifact.Spec.DiskSize != "" {
		buildCloudImageContainer.Env = []corev1.EnvVar{{
			Name:  "EXTEND",
			Value: artifact.Spec.DiskSize,
		}}
	}

	extractNetboot := corev1.Container{
		ImagePullPolicy: corev1.PullAlways,
		SecurityContext: &corev1.SecurityContext{Privileged: ptr(true)},
		Name:            "build-netboot",
		Image:           r.ToolImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Env: []corev1.EnvVar{{
			Name:  "URL",
			Value: artifact.Spec.NetbootURL,
		}},
		Args: []string{
			fmt.Sprintf(
				"/netboot.sh /artifacts/%s.iso /artifacts/%s",
				artifact.Name,
				artifact.Name,
			),
		},
		VolumeMounts: volumeMounts,
	}

	buildAzureCloudImageContainer := corev1.Container{
		ImagePullPolicy: corev1.PullAlways,
		SecurityContext: &corev1.SecurityContext{Privileged: ptr(true)},
		Name:            "build-azure-cloud-image",
		Image:           r.ToolImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			fmt.Sprintf(
				"/azure.sh /artifacts/%s.raw /artifacts/%s.vhd",
				artifact.Name,
				artifact.Name,
			),
		},
		VolumeMounts: volumeMounts,
	}

	buildGCECloudImageContainer := corev1.Container{
		ImagePullPolicy: corev1.PullAlways,
		SecurityContext: &corev1.SecurityContext{Privileged: ptr(true)},
		Name:            "build-gce-cloud-image",
		Image:           r.ToolImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			fmt.Sprintf(
				"/gce.sh /artifacts/%s.raw /artifacts/%s.gce.raw",
				artifact.Name,
				artifact.Name,
			),
		},
		VolumeMounts: volumeMounts,
	}

	podSpec := corev1.PodSpec{
		AutomountServiceAccountToken: ptr(false),
		RestartPolicy:                corev1.RestartPolicyNever,
		Volumes: []corev1.Volume{
			{
				Name: "artifacts",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
						ReadOnly:  false,
					},
				},
			},
			{
				Name:         "rootfs",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: artifact.Name,
						},
					},
				},
			},
		},
	}

	if artifact.Spec.BaseImageDockerfile != nil {
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: "dockerfile",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: artifact.Spec.BaseImageDockerfile.Name,
				},
			},
		})
	}

	if artifact.Spec.CloudConfigRef != nil {
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: "cloudconfig",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: artifact.Spec.CloudConfigRef.Name,
					Optional:   ptr(true),
				},
			},
		})
	}

	podSpec.ImagePullSecrets = append(podSpec.ImagePullSecrets, artifact.Spec.ImagePullSecrets...)

	podSpec.InitContainers = []corev1.Container{}
	// Base image can be:
	// - built from a dockerfile and converted to a kairos one
	// - built by converting an existing image to a kairos one
	// - a prebuilt kairos image
	if artifact.Spec.BaseImageDockerfile != nil {
		podSpec.InitContainers = append(podSpec.InitContainers, baseImageBuildContainers()...)
	} else if artifact.Spec.BaseImageName != "" { // Existing base image - non kairos
		podSpec.InitContainers = append(podSpec.InitContainers,
			unpackContainer("baseimage-non-kairos", r.ToolImage, artifact.Spec.BaseImageName))
	} else { // Existing Kairos base image
		podSpec.InitContainers = append(podSpec.InitContainers, unpackContainer("baseimage", r.ToolImage, artifact.Spec.ImageName))
	}

	// If base image was a non kairos one, either one we built with kaniko or prebuilt,
	// convert it to a Kairos one, in a best effort manner.
	if artifact.Spec.BaseImageDockerfile != nil || artifact.Spec.BaseImageName != "" {
		podSpec.InitContainers = append(podSpec.InitContainers,
			corev1.Container{
				ImagePullPolicy: corev1.PullAlways,
				Name:            "convert-to-kairos",
				Image:           "busybox",
				Command:         []string{"/bin/echo"},
				Args:            []string{"TODO"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "rootfs",
						MountPath: "/rootfs",
					},
				},
			})
	}

	for i, bundle := range artifact.Spec.Bundles {
		podSpec.InitContainers = append(podSpec.InitContainers, unpackContainer(fmt.Sprint(i), r.ToolImage, bundle))
	}

	if artifact.Spec.OSRelease != "" {
		podSpec.InitContainers = append(podSpec.InitContainers, osReleaseContainer(r.ToolImage))
	}

	if artifact.Spec.ISO || artifact.Spec.Netboot {
		podSpec.Containers = append(podSpec.Containers, buildIsoContainer)
	}

	if artifact.Spec.Netboot {
		podSpec.Containers = append(podSpec.Containers, extractNetboot)
	}

	if artifact.Spec.CloudImage {
		podSpec.Containers = append(podSpec.Containers, buildCloudImageContainer)
	}

	if artifact.Spec.AzureImage {
		podSpec.Containers = append(podSpec.Containers, buildAzureCloudImageContainer)
	}

	if artifact.Spec.GCEImage {
		podSpec.Containers = append(podSpec.Containers, buildGCECloudImageContainer)
	}

	podSpec.Containers = append(podSpec.Containers, createImageContainer(r.ToolImage, artifact))

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: artifact.Name + "-",
			Namespace:    artifact.Namespace,
		},
		Spec: podSpec,
	}
}

func ptr[T any](val T) *T {
	return &val
}

func baseImageBuildContainers() []corev1.Container {
	return []corev1.Container{
		{
			ImagePullPolicy: corev1.PullAlways,
			Name:            "kaniko-build",
			Image:           "gcr.io/kaniko-project/executor:latest",
			Args: []string{
				"--dockerfile", "dockerfile/Dockerfile",
				"--context", "dir://workspace",
				"--destination", "whatever", // We don't push, but it needs this
				"--tar-path", "/rootfs/image.tar",
				"--no-push",
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "rootfs",
					MountPath: "/rootfs",
				},
				{
					Name:      "dockerfile",
					MountPath: "/workspace/dockerfile",
				},
			},
		},
		{
			ImagePullPolicy: corev1.PullAlways,
			Name:            "image-extractor",
			Image:           "quay.io/luet/base",
			Args: []string{
				"util", "unpack", "--local", "file:////rootfs/image.tar", "/rootfs",
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "rootfs",
					MountPath: "/rootfs",
				},
			},
		},
		{
			ImagePullPolicy: corev1.PullAlways,
			Name:            "cleanup",
			Image:           "busybox",
			Command:         []string{"/bin/rm"},
			Args: []string{
				"/rootfs/image.tar",
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "rootfs",
					MountPath: "/rootfs",
				},
			},
		},
	}
}
