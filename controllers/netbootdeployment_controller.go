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
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	buildv1alpha1 "github.com/kairos-io/osbuilder/api/v1alpha1"
)

// NetbootDeploymentReconciler reconciles a NetbootDeployment object
type NetbootDeploymentReconciler struct {
	client.Client
	clientSet *kubernetes.Clientset

	Image  string
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=build.kairos.io,resources=netbootdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=build.kairos.io,resources=netbootdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=build.kairos.io,resources=netbootdeployments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NetbootDeployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NetbootDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var netboot buildv1alpha1.NetbootDeployment
	if err := r.Get(ctx, req.NamespacedName, &netboot); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info(fmt.Sprintf("Reconciling %v", netboot))
	// generate configmap required for building a custom image
	desiredConfigMap := r.genConfigMap(netboot)
	logger.Info(fmt.Sprintf("Checking configmap %v", netboot))

	cfgMap, err := r.clientSet.CoreV1().ConfigMaps(req.Namespace).Get(ctx, desiredConfigMap.Name, metav1.GetOptions{})
	if cfgMap == nil || apierrors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("Creating service %v", desiredConfigMap))

		cfgMap, err = r.clientSet.CoreV1().ConfigMaps(req.Namespace).Create(ctx, desiredConfigMap, metav1.CreateOptions{})
		if err != nil {
			logger.Error(err, "Failed while creating cfgmap")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	desiredDeployment := r.genDeployment(netboot)
	deployment, err := r.clientSet.AppsV1().Deployments(req.Namespace).Get(ctx, desiredDeployment.Name, metav1.GetOptions{})
	if deployment == nil || apierrors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("Creating Deployment %v", deployment))

		deployment, err = r.clientSet.AppsV1().Deployments(req.Namespace).Create(ctx, desiredDeployment, metav1.CreateOptions{})
		if err != nil {
			logger.Error(err, "Failed while creating deployment")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{Requeue: true}, nil
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetbootDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {

	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.clientSet = clientset

	return ctrl.NewControllerManagedBy(mgr).
		For(&buildv1alpha1.NetbootDeployment{}).
		Complete(r)
}

func genNetbootOwner(artifact buildv1alpha1.NetbootDeployment) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(&artifact.ObjectMeta, schema.GroupVersionKind{
			Group:   buildv1alpha1.GroupVersion.Group,
			Version: buildv1alpha1.GroupVersion.Version,
			Kind:    "NetbootDeployment",
		}),
	}
}

func (r *NetbootDeploymentReconciler) genConfigMap(artifact buildv1alpha1.NetbootDeployment) *v1.ConfigMap {
	return &v1.ConfigMap{

		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s-netboot", artifact.Name),
			Namespace:       artifact.Namespace,
			OwnerReferences: genNetbootOwner(artifact),
		},
		Data: map[string]string{
			"cloudconfig": artifact.Spec.CloudConfig,
		}}
}
