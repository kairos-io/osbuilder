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
	"bytes"
	"context"
	"fmt"

	buildv1alpha1 "github.com/kairos-io/osbuilder/api/v1alpha1"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

func genJobLabel(s string) map[string]string {
	return map[string]string{
		"osbuild": "workload" + s,
	}
}

// TODO: Handle registry auth
// TODO: This shells out, but needs ENV_VAR with key refs mapping
// TODO: Cache downloaded images?
func unpackContainer(id, containerImage, pullImage string, pullOptions buildv1alpha1.Pull) v1.Container {
	return v1.Container{
		ImagePullPolicy: v1.PullAlways,
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
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "rootfs",
				MountPath: "/rootfs",
			},
		},
	}
}

func createImageContainer(containerImage string, pushOptions buildv1alpha1.Push) v1.Container {
	return v1.Container{
		ImagePullPolicy: v1.PullAlways,
		Name:            "create-image",
		Image:           containerImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			fmt.Sprintf(
				"tar -czvpf test.tar -C /rootfs . && luet util pack %s test.tar image.tar && mv image.tar /artifacts",
				pushOptions.ImageName,
			),
		},
		VolumeMounts: []v1.VolumeMount{
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

func createPushToServerImageContainer(containerImage string, artifactPodInfo ArtifactPodInfo) v1.Container {
	command := fmt.Sprintf("tar cf - -C artifacts/ . | kubectl exec -i -n %s $(kubectl  get pods -l %s -n %s --no-headers -o custom-columns=\":metadata.name\" | head -n1) -- tar xf - -C %s", artifactPodInfo.Namespace, artifactPodInfo.Label, artifactPodInfo.Namespace, artifactPodInfo.Path)
	fmt.Printf("command = %+v\n", command)

	return v1.Container{
		ImagePullPolicy: v1.PullAlways,
		Name:            "push-to-server",
		Image:           containerImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args:            []string{command},
		VolumeMounts: []v1.VolumeMount{
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

func osReleaseContainer(containerImage string) v1.Container {
	return v1.Container{
		ImagePullPolicy: v1.PullAlways,
		Name:            "os-release",
		Image:           containerImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			"cp -rfv /etc/os-release /rootfs/etc/os-release",
		},
		VolumeMounts: []v1.VolumeMount{
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

func (r *OSArtifactReconciler) genJob(artifact buildv1alpha1.OSArtifact) *batchv1.Job {
	objMeta := genObjectMeta(artifact)

	pushImage := artifact.Spec.PushOptions.Push

	privileged := false
	serviceAccount := true

	cmd := fmt.Sprintf(
		"/entrypoint.sh --debug --name %s build-iso --date=false --output /artifacts dir:/rootfs",
		artifact.Name,
	)

	volumeMounts := []v1.VolumeMount{
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
		volumeMounts = append(volumeMounts, v1.VolumeMount{
			Name:      "config",
			MountPath: "/iso/iso-overlay/boot/grub2/grub.cfg",
			SubPath:   "grub.cfg",
		})
	}

	cloudImgCmd := fmt.Sprintf(
		"/raw-images.sh /rootfs /artifacts/%s.raw",
		artifact.Name,
	)

	if artifact.Spec.CloudConfig != "" {
		volumeMounts = append(volumeMounts, v1.VolumeMount{
			Name:      "config",
			MountPath: "/iso/iso-overlay/cloud_config.yaml",
			SubPath:   "config",
		})

		cloudImgCmd += " /iso/iso-overlay/cloud_config.yaml"
	}

	if artifact.Spec.CloudConfig != "" || artifact.Spec.GRUBConfig != "" {
		cmd = fmt.Sprintf(
			"/entrypoint.sh --debug --name %s build-iso --date=false --overlay-iso /iso/iso-overlay --output /artifacts dir:/rootfs",
			artifact.Name,
		)
	}

	buildIsoContainer := v1.Container{
		ImagePullPolicy: v1.PullAlways,
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
		Name:            "build-iso",
		Image:           r.ToolImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Args: []string{
			cmd,
		},
		VolumeMounts: volumeMounts,
	}

	buildCloudImageContainer := v1.Container{
		ImagePullPolicy: v1.PullAlways,
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
		Name:            "build-cloud-image",
		Image:           r.ToolImage,

		Command: []string{"/bin/bash", "-cxe"},
		Args: []string{
			cloudImgCmd,
		},
		VolumeMounts: volumeMounts,
	}

	if artifact.Spec.DiskSize != "" {
		buildCloudImageContainer.Env = []v1.EnvVar{{
			Name:  "EXTEND",
			Value: artifact.Spec.DiskSize,
		}}
	}

	extractNetboot := v1.Container{
		ImagePullPolicy: v1.PullAlways,
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
		Name:            "build-netboot",
		Image:           r.ToolImage,
		Command:         []string{"/bin/bash", "-cxe"},
		Env: []v1.EnvVar{{
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

	buildAzureCloudImageContainer := v1.Container{
		ImagePullPolicy: v1.PullAlways,
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
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

	buildGCECloudImageContainer := v1.Container{
		ImagePullPolicy: v1.PullAlways,
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
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

	pod := v1.PodSpec{
		AutomountServiceAccountToken: &serviceAccount,
		ServiceAccountName:           objMeta.Name,
		RestartPolicy:                v1.RestartPolicyNever,
		Volumes: []v1.Volume{
			{
				Name:         "artifacts",
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

	pod.InitContainers = []v1.Container{unpackContainer("baseimage", r.ToolImage, artifact.Spec.ImageName, artifact.Spec.PullOptions)}

	for i, bundle := range artifact.Spec.Bundles {
		pod.InitContainers = append(pod.InitContainers, unpackContainer(fmt.Sprint(i), r.ToolImage, bundle, artifact.Spec.PullOptions))
	}

	if artifact.Spec.OSRelease != "" {
		pod.InitContainers = append(pod.InitContainers, osReleaseContainer(r.ToolImage))
	}

	if artifact.Spec.ISO || artifact.Spec.Netboot {
		pod.InitContainers = append(pod.InitContainers, buildIsoContainer)
	}

	if artifact.Spec.Netboot {
		pod.InitContainers = append(pod.InitContainers, extractNetboot)
	}

	if artifact.Spec.CloudImage || artifact.Spec.AzureImage || artifact.Spec.GCEImage {
		pod.InitContainers = append(pod.InitContainers, buildCloudImageContainer)
	}

	if artifact.Spec.AzureImage {
		pod.InitContainers = append(pod.InitContainers, buildAzureCloudImageContainer)
	}

	if artifact.Spec.GCEImage {
		pod.InitContainers = append(pod.InitContainers, buildGCECloudImageContainer)
	}

	// TODO: Shell out to `kubectl cp`? Why not?
	// TODO: Does it make sense to build the image and not push it? Maybe remove
	// this flag?
	if pushImage {
		pod.InitContainers = append(pod.InitContainers, createImageContainer(r.ToolImage, artifact.Spec.PushOptions))
	}

	pod.Containers = []v1.Container{
		// TODO: Add kubectl to osbuilder-tools?
		//createPushToServerImageContainer(r.ToolImage),
		createPushToServerImageContainer("bitnami/kubectl", r.ArtifactPodInfo),
	}

	jobLabels := genJobLabel(artifact.Name)

	job := batchv1.Job{
		ObjectMeta: objMeta,
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: jobLabels,
				},
				Spec: pod,
			},
		},
	}

	return &job
}

// createServiceAccount creates a service account that has the permissions to
// copy the artifacts to the http server Pod. This service account is used for
// the "push to server" container.
func (r *OSArtifactReconciler) createCopierServiceAccount(ctx context.Context, objMeta metav1.ObjectMeta) error {
	sa, err := r.clientSet.CoreV1().
		ServiceAccounts(objMeta.Namespace).Get(ctx, objMeta.Name, metav1.GetOptions{})
	if sa == nil || apierrors.IsNotFound(err) {
		t := true
		_, err := r.clientSet.CoreV1().ServiceAccounts(objMeta.Namespace).Create(ctx,
			&v1.ServiceAccount{
				ObjectMeta:                   objMeta,
				AutomountServiceAccountToken: &t,
			}, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}

	return err
}

// func (r *OSArtifactReconciler) createCopierRole(ctx context.Context, objMeta metav1.ObjectMeta) error {
// 	role, err := r.clientSet.RbacV1().
// 		Roles(objMeta.Namespace).
// 		Get(ctx, objMeta.Name, metav1.GetOptions{})
// 	if role == nil || apierrors.IsNotFound(err) {
// 		_, err := r.clientSet.RbacV1().Roles(objMeta.Namespace).Create(ctx,
// 			&rbacv1.Role{
// 				ObjectMeta: objMeta,
// 				Rules: []rbacv1.PolicyRule{
// 					// TODO: The actual permissions we need is that to copy to a Pod.
// 					// The Pod is on another namespace, so we need a cluster wide permission.
// 					// This can get viral because the controller needs to have the permissions
// 					// if it is to grant them to the Job.
// 					{
// 						Verbs:     []string{"list"},
// 						APIGroups: []string{""},
// 						Resources: []string{"pods"},
// 					},
// 				},
// 			},
// 			metav1.CreateOptions{},
// 		)
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return err
// }

func (r *OSArtifactReconciler) createCopierRoleBinding(ctx context.Context, objMeta metav1.ObjectMeta) error {
	newrb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objMeta.Name,
			Namespace: r.ArtifactPodInfo.Namespace,
			// TODO: We can't have cross-namespace owners. The role binding will have to deleted explicitly by the reconciler (finalizer?)
			// OwnerReferences: objMeta.OwnerReferences,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     r.ArtifactPodInfo.Role,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      objMeta.Name,
				Namespace: objMeta.Namespace,
			},
		},
	}

	rb, err := r.clientSet.RbacV1().
		RoleBindings(r.ArtifactPodInfo.Namespace).
		Get(ctx, objMeta.Name, metav1.GetOptions{})
	if rb == nil || apierrors.IsNotFound(err) {
		_, err := r.clientSet.RbacV1().
			RoleBindings(r.ArtifactPodInfo.Namespace).
			Create(ctx, newrb, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}

	return err
}

// createRBAC creates a ServiceAccount, and a binding to the CopierRole so that
// the container that copies the artifacts to the http server Pod has the
// permissions to do so.
func (r *OSArtifactReconciler) createRBAC(ctx context.Context, artifact buildv1alpha1.OSArtifact) error {
	objMeta := genObjectMeta(artifact)

	err := r.createCopierServiceAccount(ctx, objMeta)
	if err != nil {
		return errors.Wrap(err, "creating a service account")
	}

	err = r.createCopierRoleBinding(ctx, objMeta)
	if err != nil {
		return errors.Wrap(err, "creating a role binding for the copy-role")
	}

	return err
}

// removeRBAC deletes the role binding between the service account of this artifact
// and the CopierRole. The ServiceAccount is removed automatically through the Owner
// relationship with the OSArtifact. The RoleBinding can't have it as an owner
// because it is in a different Namespace.
func (r *OSArtifactReconciler) removeRBAC(ctx context.Context, artifact buildv1alpha1.OSArtifact) error {
	err := r.clientSet.RbacV1().RoleBindings(r.ArtifactPodInfo.Namespace).
		Delete(ctx, artifact.Name, metav1.DeleteOptions{})
	// Ignore not found. No need to do anything.
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	}

	return err
}

func (r *OSArtifactReconciler) removeArtifacts(ctx context.Context, artifact buildv1alpha1.OSArtifact) error {
	//Finding Pods using labels
	fmt.Printf("r.ArtifactPodInfo = %+v\n", r.ArtifactPodInfo.Label)
	pods, err := r.clientSet.CoreV1().Pods(r.ArtifactPodInfo.Namespace).
		List(ctx, metav1.ListOptions{LabelSelector: r.ArtifactPodInfo.Label})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("listing pods with label %s in namespace %s", r.ArtifactPodInfo.Label, r.ArtifactPodInfo.Namespace))
	}
	if len(pods.Items) < 1 {
		return errors.New("No artifact pod found")
	}
	pod := pods.Items[0]

	stdout, stderr, err := r.executeRemoteCommand(r.ArtifactPodInfo.Namespace, pod.Name, fmt.Sprintf("rm -rf %s/%s.*", r.ArtifactPodInfo.Path, artifact.Name))
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s\n%s", stdout, stderr))
	}
	return nil
}

func (r *OSArtifactReconciler) executeRemoteCommand(namespace, podName, command string) (string, string, error) {
	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	request := r.clientSet.CoreV1().RESTClient().
		Post().
		Namespace(namespace).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Command: []string{"/bin/sh", "-c", command},
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(r.restConfig, "POST", request.URL())
	if err != nil {
		return "", "", err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: buf,
		Stderr: errBuf,
	})
	if err != nil {
		return "", "", fmt.Errorf("%w Failed executing command %s on %v/%v", err, command, namespace, podName)
	}

	return buf.String(), errBuf.String(), nil
}
