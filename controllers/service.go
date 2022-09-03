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
	buildv1alpha1 "github.com/c3os-io/osbuilder-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func genService(artifact buildv1alpha1.OSArtifact) *v1.Service {
	objMeta := metav1.ObjectMeta{
		Name:            artifact.Name,
		Namespace:       artifact.Namespace,
		OwnerReferences: genOwner(artifact),
	}
	return &v1.Service{
		ObjectMeta: objMeta,
		Spec: v1.ServiceSpec{
			Ports:    []v1.ServicePort{{Name: "http", Port: int32(80), TargetPort: intstr.FromInt(80)}},
			Selector: genDeploymentLabel(artifact.Name),
		},
	}
}
