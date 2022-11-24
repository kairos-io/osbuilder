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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NetbootDeploymentSpec defines the desired state of NetbootDeployment
type NetbootDeploymentSpec struct {
	CloudConfig string `json:"cloudConfig,omitempty"`
	CommandLine string `json:"cmdLine,omitempty"`

	// TODO: Those below should be deprecated and reference an osbuild instead
}

// NetbootDeploymentStatus defines the observed state of NetbootDeployment
type NetbootDeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NetbootDeployment is the Schema for the netbootdeployments API
type NetbootDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetbootDeploymentSpec   `json:"spec,omitempty"`
	Status NetbootDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NetbootDeploymentList contains a list of NetbootDeployment
type NetbootDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetbootDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetbootDeployment{}, &NetbootDeploymentList{})
}
