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
	"time"

	buildv1alpha1 "github.com/c3os-io/osbuilder-operator/api/v1alpha1"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// OSArtifactReconciler reconciles a OSArtifact object
type OSArtifactReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	clientSet               *kubernetes.Clientset
	ServingImage, ToolImage string
}

func genOwner(artifact buildv1alpha1.OSArtifact) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(&artifact.ObjectMeta, schema.GroupVersionKind{
			Group:   buildv1alpha1.GroupVersion.Group,
			Version: buildv1alpha1.GroupVersion.Version,
			Kind:    "OSArtifact",
		}),
	}
}

//+kubebuilder:rbac:groups=build.c3os-x.io,resources=osartifacts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=build.c3os-x.io,resources=osartifacts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=build.c3os-x.io,resources=osartifacts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OSArtifact object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *OSArtifactReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var osbuild buildv1alpha1.OSArtifact
	if err := r.Get(ctx, req.NamespacedName, &osbuild); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info(fmt.Sprintf("Reconciling %v", osbuild))

	// generate configmap required for building a custom image
	desiredConfigMap := r.genConfigMap(osbuild)
	logger.Info(fmt.Sprintf("Checking configmap %v", osbuild))

	cfgMap, err := r.clientSet.CoreV1().ConfigMaps(req.Namespace).Get(ctx, desiredConfigMap.Name, v1.GetOptions{})
	if cfgMap == nil || apierrors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("Creating service %v", desiredConfigMap))

		cfgMap, err = r.clientSet.CoreV1().ConfigMaps(req.Namespace).Create(ctx, desiredConfigMap, v1.CreateOptions{})
		if err != nil {
			logger.Error(err, "Failed while creating svc")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	desiredService := genService(osbuild)
	logger.Info(fmt.Sprintf("Checking service %v", osbuild))

	svc, err := r.clientSet.CoreV1().Services(req.Namespace).Get(ctx, desiredService.Name, v1.GetOptions{})
	if svc == nil || apierrors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("Creating service %v", desiredService))

		svc, err = r.clientSet.CoreV1().Services(req.Namespace).Create(ctx, desiredService, v1.CreateOptions{})
		if err != nil {
			logger.Error(err, "Failed while creating svc")
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	logger.Info(fmt.Sprintf("Checking deployment %v", osbuild))

	desiredDeployment := r.genDeployment(osbuild)
	deployment, err := r.clientSet.AppsV1().Deployments(req.Namespace).Get(ctx, desiredDeployment.Name, v1.GetOptions{})
	if deployment == nil || apierrors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("Creating Deployment %v", deployment))

		deployment, err = r.clientSet.AppsV1().Deployments(req.Namespace).Create(ctx, desiredDeployment, v1.CreateOptions{})
		if err != nil {
			logger.Error(err, "Failed while creating deployment")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{Requeue: true}, nil
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	logger.Info(fmt.Sprintf("Updating state %v", osbuild))

	copy := osbuild.DeepCopy()

	helper, err := patch.NewHelper(&osbuild, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	if deployment.Status.ReadyReplicas == deployment.Status.Replicas {
		copy.Status.Phase = "Ready"
	} else if copy.Status.Phase != "Building" {
		copy.Status.Phase = "Building"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	if err := helper.Patch(ctx, copy); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "couldn't patch osbuild %q", copy.Name)
	}

	// for _, c := range append(pod.Status.ContainerStatuses, pod.Status.InitContainerStatuses...) {
	// 	if c.State.Terminated != nil && c.State.Terminated.ExitCode != 0 {
	// 		packageBuildCopy.Status.State = "Failed"
	// 	}
	// }

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OSArtifactReconciler) SetupWithManager(mgr ctrl.Manager) error {

	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.clientSet = clientset

	return ctrl.NewControllerManagedBy(mgr).
		For(&buildv1alpha1.OSArtifact{}).
		Complete(r)
}
