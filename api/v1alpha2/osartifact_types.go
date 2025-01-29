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

	CloudConfigRef *SecretKeySelector `json:"cloudConfigRef,omitempty"`

	Bundles     []string          `json:"bundles,omitempty"`
	FileBundles map[string]string `json:"fileBundles,omitempty"`

	OutputImage *OutputImage `json:"outputImage,omitempty"`
}

type SecretKeySelector struct {
	Name string `json:"name"`
	// +optional
	Key string `json:"key,omitempty"`
}

type RegistryCloud string

const (
	// RegistryCloudECR ensures that special env variables will be injected
	// into the exporter job to allow kaniko to automatically auth with the
	// ecr registry to push the images.
	RegistryCloudECR RegistryCloud = "ecr"
	// RegistryCloudOther requires from user to provide username/password secret
	// in order for kaniko to be able to authenticate with the container registry.
	RegistryCloudOther RegistryCloud = "other"
)

type OutputImage struct {
	// +kubebuilder:validation:Enum=ecr;other
	// +kubebuilder:default=other
	// +required
	Cloud RegistryCloud `json:"cloud"`
	// +optional
	Registry string `json:"registry,omitempty"`
	// +optional
	Repository string `json:"repository,omitempty"`
	// +optional
	Tag string `json:"tag,omitempty"`
	// +optional
	DockerConfigSecretKeyRef *SecretKeySelector `json:"dockerConfigSecretKeyRef,omitempty"`
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
