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

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Model string

const (
	RPI3 Model = "rpi3"
	RPI4 Model = "rpi4"
)

// OSArtifactSpec defines the desired state of OSArtifact
type OSArtifactSpec struct {
	// There are 3 ways to specify a Kairos image:

	// Points to a prepared kairos image (e.g. a released one)
	ImageName string `json:"imageName,omitempty"`

	ISO bool `json:"iso,omitempty"`

	// +kubebuilder:validation:Type:=string
	// +kubebuilder:validation:Enum:=rpi3;rpi4
	Model *Model `json:"model,omitempty"`

	// +optional
	CloudConfigRef *corev1.SecretKeySelector `json:"cloudConfigRef,omitempty"`

	// +optional
	Bundles []string `json:"bundles,omitempty"`

	// +optional
	FileBundles map[string]string `json:"fileBundles,omitempty"`

	// Exporter when provided it will spawn an exporter job that
	// pushes images built by the osbuilder to the provided registry.
	// +optional
	Exporter *ExporterSpec `json:"exporter,omitempty"`
}

type RegistryType string

const (
	// RegistryTypeECR ensures that special env variables will be injected
	// into the exporter job to allow kaniko to automatically auth with the
	// ecr registry to push the images.
	RegistryTypeECR RegistryType = "ecr"
	// RegistryTypeOther requires from user to provide username/password secret
	// in order for kaniko to be able to authenticate with the container registry.
	RegistryTypeOther RegistryType = "other"
)

type ExporterSpec struct {
	// Registry is a registry spec used to push the final images built by the osbuilder.
	// +required
	Registry RegistrySpec `json:"registry"`

	// ServiceAccount allows overriding 'default' SA bound to the exporter pods.
	// +optional
	ServiceAccount *string `json:"serviceAccount,omitempty"`

	// ExtraEnvVars allows to append extra env vars to the exporter pods.
	// +optional
	ExtraEnvVars *[]corev1.EnvVar `json:"extraEnvVars,omitempty"`

	// Image is the image used for exporter pods
	// +optional
	Image string `json:"image,omitempty"`

	// ExtraArgs allows appending args to the exporter image.
	// +optional
	ExtraArgs []string `json:"extraArgs,omitempty"`
}

func (in *ExporterSpec) IsECRRegistry() bool {
	return in.Registry.Type == RegistryTypeECR
}

func (in *ExporterSpec) HasDockerConfigSecret() bool {
	return in.Registry.DockerConfigSecretKeyRef != nil
}

func (in *ExporterSpec) HasExtraEnvVars() bool {
	return in.ExtraEnvVars != nil && len(*in.ExtraEnvVars) > 0
}

func (in *ExporterSpec) ServiceAccountName() string {
	if in.ServiceAccount == nil || len(*in.ServiceAccount) == 0 {
		// Default SA name. Always exists.
		return "default"
	}

	return *in.ServiceAccount
}

type ImageSpec struct {
	// Repository is the name of repository where image is being pushed.
	// +required
	Repository string `json:"repository"`

	// Tag is the tag name of the image being pushed. Defaults to 'latest' if not provided.
	// +optional
	Tag string `json:"tag,omitempty"`
}

type RegistrySpec struct {
	// Name is a DNS name of the registry. It has to be accessible by the pod.
	// +required
	Name string `json:"name"`

	// Type is a kind of registry being used. Currently supported values are:
	// 	- ecr 	- Amazon Elastic Container Registry. Use only if a pod runs on
	//			  an eks cluster and has permissions to push to the registry.
	//	- other - Any other type of the registry. It requires DockerConfigSecretKeyRef
	//			  to be provided in order to auth to the registry.
	// +kubebuilder:validation:Enum=ecr;other
	// +kubebuilder:default=other
	// +required
	Type RegistryType `json:"type"`

	// Image defines the image details required to push image to the registry.
	// +required
	Image ImageSpec `json:"image"`

	// DockerConfigSecretKeyRef is a reference to the secret that holds the `config.json` auth file.
	// It should be in a format that `docker login` can accept to auth to the registry.
	// +optional
	DockerConfigSecretKeyRef *corev1.SecretKeySelector `json:"dockerConfigSecretKeyRef,omitempty"`
}

type ArtifactPhase string

const (
	Pending   = "Pending"
	Building  = "Building"
	Exporting = "Exporting"
	Ready     = "Ready"
	Error     = "Error"
)

// OSArtifactStatus defines the observed state of OSArtifact
type OSArtifactStatus struct {
	// +kubebuilder:default=Pending
	Phase ArtifactPhase `json:"phase,omitempty"`

	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// OSArtifact is the Schema for the osartifacts API
type OSArtifact struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OSArtifactSpec   `json:"spec,omitempty"`
	Status OSArtifactStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OSArtifactList contains a list of OSArtifact
type OSArtifactList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OSArtifact `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OSArtifact{}, &OSArtifactList{})
}
