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

	osbuilder "github.com/kairos-io/osbuilder/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				Name:      "artifacts",
				MountPath: "/rootfs",
				SubPath:   "rootfs",
			},
		},
	}
}

func unpackFileContainer(id, pullImage, name string) corev1.Container {
	return corev1.Container{
		ImagePullPolicy: corev1.PullAlways,
		Name:            fmt.Sprintf("pull-image-%s", id),
		Image:           "gcr.io/go-containerregistry/crane:latest",
		Command:         []string{"crane"},
		Args:            []string{"--platform=linux/arm64", "pull", pullImage, fmt.Sprintf("/rootfs/%s.tar", name)},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "artifacts",
				MountPath: "/rootfs",
				SubPath:   "rootfs",
			},
		},
	}
}

func (r *OSArtifactReconciler) newArtifactPVC(artifact *osbuilder.OSArtifact) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      artifact.Name + "-artifacts",
			Namespace: artifact.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					"storage": resource.MustParse(r.PVCStorage),
				},
			},
		},
	}

	return pvc
}

func (r *OSArtifactReconciler) newBuilderPod(pvcName string, artifact *osbuilder.OSArtifact) *corev1.Pod {
	cmd := fmt.Sprintf(
		"auroraboot --debug build-iso --name %s --date=false --output /artifacts dir:/rootfs",
		artifact.Name,
	)

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "artifacts",
			MountPath: "/artifacts",
			SubPath:   "artifacts",
		},
		{
			Name:      "artifacts",
			MountPath: "/rootfs",
			SubPath:   "rootfs",
		},
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

	if artifact.Spec.Model != nil {
		cmd = fmt.Sprintf("/build-arm-image.sh --model %s --directory %s --recovery-partition-size 5120 --state-parition-size 6144 --size 16384 --images-size 4096 /artifacts/%s.iso", *artifact.Spec.Model, "/rootfs", artifact.Name)
		if artifact.Spec.CloudConfigRef != nil {
			cmd = fmt.Sprintf("/build-arm-image.sh --model %s --config /iso/iso-overlay/cloud_config.yaml --directory %s /artifacts/%s.iso", *artifact.Spec.Model, "/rootfs", artifact.Name)
		}
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

	if artifact.Spec.ISO && artifact.Spec.Model != nil {
		podSpec.InitContainers = []corev1.Container{}

		podSpec.InitContainers = append(podSpec.InitContainers, corev1.Container{
			Name:    "create-directories",
			Image:   "busybox",
			Command: []string{"sh", "-c", "mkdir -p /mnt/pv/artifacts && chown -R 65532:65532 /mnt/pv/artifacts && mkdir -p /mnt/pv/rootfs && chown -R 65532:65532 /mnt/pv/rootfs"},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "artifacts",
					MountPath: "/mnt/pv",
				},
			},
		})

		i := 0
		for name, bundle := range artifact.Spec.FileBundles {
			i++
			podSpec.InitContainers = append(podSpec.InitContainers, unpackFileContainer(fmt.Sprint(i), bundle, name))
		}
		for i, bundle := range artifact.Spec.Bundles {
			podSpec.InitContainers = append(podSpec.InitContainers, unpackContainer(fmt.Sprint(i), r.ToolImage, bundle))
		}
		podSpec.InitContainers = append(podSpec.InitContainers, unpackContainer("baseimage", r.ToolImage, artifact.Spec.ImageName))
		podSpec.Containers = make([]corev1.Container, 0)

		podSpec.Containers = append(podSpec.Containers, buildIsoContainer)
	}

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
