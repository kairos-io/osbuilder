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

	buildv1alpha1 "github.com/kairos-io/osbuilder/api/v1alpha1"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const FinalizerName = "build.kairos.io/osbuilder-finalizer"

type ArtifactPodInfo struct {
	Label     string
	Namespace string
	Path      string
	Role      string
}

// OSArtifactReconciler reconciles a OSArtifact object
type OSArtifactReconciler struct {
	client.Client
	Scheme                               *runtime.Scheme
	restConfig                           *rest.Config
	clientSet                            *kubernetes.Clientset
	ServingImage, ToolImage, CopierImage string
	ArtifactPodInfo                      ArtifactPodInfo
}

func genObjectMeta(artifact buildv1alpha1.OSArtifact) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            artifact.Name,
		Namespace:       artifact.Namespace,
		OwnerReferences: genOwner(artifact),
	}
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

//+kubebuilder:rbac:groups=build.kairos.io,resources=osartifacts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=build.kairos.io,resources=osartifacts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=build.kairos.io,resources=osartifacts/finalizers,verbs=update

// TODO: Is this ^ how I should have created rbac permissions for the controller?
//       - git commit all changes
//       - generate code with kubebuilder
//       - check if my permissions were removed
//       - do it properly

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

	stop, err := r.handleFinalizer(ctx, &osbuild)
	if err != nil || stop {
		return ctrl.Result{}, err
	}

	// generate configmap required for building a custom image
	desiredConfigMap := r.genConfigMap(osbuild)
	logger.Info(fmt.Sprintf("Checking configmap %v", osbuild))

	cfgMap, err := r.clientSet.CoreV1().ConfigMaps(req.Namespace).Get(ctx, desiredConfigMap.Name, metav1.GetOptions{})
	if cfgMap == nil || apierrors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("Creating config map %v", desiredConfigMap))

		_, err = r.clientSet.CoreV1().ConfigMaps(req.Namespace).Create(ctx, desiredConfigMap, metav1.CreateOptions{})
		if err != nil {
			logger.Error(err, "Failed while creating config map")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	logger.Info(fmt.Sprintf("Checking deployment %v", osbuild))

	err = r.createRBAC(ctx, osbuild)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	desiredJob := r.genJob(osbuild)
	job, err := r.clientSet.BatchV1().Jobs(req.Namespace).Get(ctx, desiredJob.Name, metav1.GetOptions{})
	if job == nil || apierrors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("Creating Job %v", job))

		_, err = r.clientSet.BatchV1().Jobs(req.Namespace).Create(ctx, desiredJob, metav1.CreateOptions{})
		if err != nil {
			logger.Error(err, "Failed while creating job")
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
	if job.Status.Succeeded > 0 {
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

	cfg := mgr.GetConfig()
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	r.restConfig = cfg
	r.clientSet = clientset

	return ctrl.NewControllerManagedBy(mgr).
		For(&buildv1alpha1.OSArtifact{}).
		Complete(r)
}

// Returns true if reconciliation should stop or false otherwise
func (r *OSArtifactReconciler) handleFinalizer(ctx context.Context, osbuild *buildv1alpha1.OSArtifact) (bool, error) {
	// examine DeletionTimestamp to determine if object is under deletion
	if osbuild.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(osbuild, FinalizerName) {
			controllerutil.AddFinalizer(osbuild, FinalizerName)
			if err := r.Update(ctx, osbuild); err != nil {
				return true, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(osbuild, FinalizerName) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.finalize(ctx, osbuild); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return true, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(osbuild, FinalizerName)
			if err := r.Update(ctx, osbuild); err != nil {
				return true, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return true, nil
	}

	return false, nil
}

// - Remove artifacts from the server Pod
// - Delete role-binding (because it doesn't have the OSArtifact as an owner and won't be deleted automatically)
func (r *OSArtifactReconciler) finalize(ctx context.Context, osbuild *buildv1alpha1.OSArtifact) error {
	if err := r.removeRBAC(ctx, *osbuild); err != nil {
		return err
	}

	if err := r.removeArtifacts(ctx, *osbuild); err != nil {
		return err
	}

	return nil
}
