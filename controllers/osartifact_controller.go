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

	"k8s.io/apimachinery/pkg/api/meta"

	"k8s.io/apimachinery/pkg/api/errors"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	osbuilder "github.com/kairos-io/osbuilder/api/v1alpha2"
)

const (
	requeueAfter                    = 5 * time.Second
	FinalizerName                   = "build.kairos.io/osbuilder-finalizer"
	artifactLabel                   = "build.kairos.io/artifact"
	artifactExporterIndexAnnotation = "build.kairos.io/export-index"
	ready                           = "Ready"
)
const threeHours = int32(10800)

var (
	requeue = ctrl.Result{RequeueAfter: requeueAfter}
)

// OSArtifactReconciler reconciles a OSArtifact object
type OSArtifactReconciler struct {
	client.Client
	Scheme                                           *runtime.Scheme
	ServingImage, ToolImage, CopierImage, PVCStorage string
}

//+kubebuilder:rbac:groups=build.kairos.io,resources=osartifacts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=build.kairos.io,resources=osartifacts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=build.kairos.io,resources=osartifacts/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;create;delete;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;create;
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete

func (r *OSArtifactReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	artifact := new(osbuilder.OSArtifact)
	if err := r.Get(ctx, req.NamespacedName, artifact); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if len(artifact.Status.Conditions) == 0 {
		artifact.Status.Conditions = []metav1.Condition{}
		meta.SetStatusCondition(&artifact.Status.Conditions, metav1.Condition{
			Type:   ready,
			Reason: ready,
			Status: metav1.ConditionFalse,
		})
		if err := TryToUpdateStatus(ctx, r.Client, artifact); err != nil {
			return ctrl.Result{}, err
		}
	}

	if artifact.DeletionTimestamp != nil {
		controllerutil.RemoveFinalizer(artifact, FinalizerName)
		return ctrl.Result{}, r.Update(ctx, artifact)
	}

	if !controllerutil.ContainsFinalizer(artifact, FinalizerName) {
		controllerutil.AddFinalizer(artifact, FinalizerName)
		if err := r.Update(ctx, artifact); err != nil {
			return ctrl.Result{Requeue: true}, err
		}
	}

	logger.Info(fmt.Sprintf("Reconciling %s/%s", artifact.Namespace, artifact.Name))

	switch artifact.Status.Phase {
	case osbuilder.Exporting:
		return r.checkExport(ctx, artifact)
	case osbuilder.Ready:
		meta.SetStatusCondition(&artifact.Status.Conditions, metav1.Condition{
			Type:   ready,
			Reason: ready,
			Status: metav1.ConditionTrue,
		})
		return ctrl.Result{}, TryToUpdateStatus(ctx, r.Client, artifact)
	case osbuilder.Error:
		return ctrl.Result{}, nil
	default:
		return r.checkBuild(ctx, artifact)
	}
}

func (r *OSArtifactReconciler) createPVC(ctx context.Context, artifact *osbuilder.OSArtifact) (*corev1.PersistentVolumeClaim, error) {
	pvc := r.newArtifactPVC(artifact)
	if pvc.Labels == nil {
		pvc.Labels = map[string]string{}
	}
	pvc.Labels[artifactLabel] = artifact.Name
	if err := controllerutil.SetOwnerReference(artifact, pvc, r.Scheme); err != nil {
		return pvc, err
	}
	if err := r.Create(ctx, pvc); err != nil {
		return pvc, err
	}

	return pvc, nil
}

func (r *OSArtifactReconciler) createBuilderPod(ctx context.Context, artifact *osbuilder.OSArtifact, pvc *corev1.PersistentVolumeClaim) (*corev1.Pod, error) {
	pod := r.newBuilderPod(pvc.Name, artifact)
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels[artifactLabel] = artifact.Name
	if err := controllerutil.SetOwnerReference(artifact, pod, r.Scheme); err != nil {
		return pod, err
	}

	if err := r.Create(ctx, pod); err != nil {
		return pod, err
	}

	return pod, nil
}

func (r *OSArtifactReconciler) startBuild(ctx context.Context, artifact *osbuilder.OSArtifact) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if artifact.Spec.CloudConfigRef != nil {
		if err := r.Get(ctx, client.ObjectKey{Namespace: artifact.Namespace, Name: artifact.Spec.CloudConfigRef.Name}, &corev1.Secret{}); err != nil {
			if errors.IsNotFound(err) {
				logger.Info(fmt.Sprintf("Secret %s/%s not found", artifact.Namespace, artifact.Spec.CloudConfigRef.Name))
				return requeue, nil
			}
			return ctrl.Result{}, err
		}
	}

	pvc, err := r.createPVC(ctx, artifact)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	_, err = r.createBuilderPod(ctx, artifact, pvc)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	artifact.Status.Phase = osbuilder.Building
	if err := TryToUpdateStatus(ctx, r.Client, artifact); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OSArtifactReconciler) checkBuild(ctx context.Context, artifact *osbuilder.OSArtifact) (ctrl.Result, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			artifactLabel: artifact.Name,
		}),
	}); err != nil {
		return ctrl.Result{}, err
	}

	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			artifact.Status.Phase = osbuilder.Exporting
			return ctrl.Result{}, TryToUpdateStatus(ctx, r.Client, artifact)
		case corev1.PodFailed:
			artifact.Status.Phase = osbuilder.Error
			meta.SetStatusCondition(&artifact.Status.Conditions, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionFalse,
				Reason:  "Error",
				Message: getCorev1PodHealth(&pod).Message,
			})
			return ctrl.Result{}, TryToUpdateStatus(ctx, r.Client, artifact)
		case corev1.PodPending, corev1.PodRunning:
			return ctrl.Result{}, nil
		}
	}

	return r.startBuild(ctx, artifact)
}

func (r *OSArtifactReconciler) checkExport(ctx context.Context, artifact *osbuilder.OSArtifact) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var jobs batchv1.JobList
	if err := r.List(ctx, &jobs, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			artifactLabel: artifact.Name,
		}),
	}); err != nil {
		log.FromContext(ctx).Error(err, "failed to list jobs")
		return ctrl.Result{Requeue: true}, nil
	}

	indexedJobs := make(map[string]*batchv1.Job, 1)
	for _, job := range jobs.Items {
		if job.GetAnnotations() != nil {
			if idx, ok := job.GetAnnotations()[artifactExporterIndexAnnotation]; ok {
				indexedJobs[idx] = &job
			}
		}
	}

	var pvcs corev1.PersistentVolumeClaimList
	var pvc *corev1.PersistentVolumeClaim
	if err := r.List(ctx, &pvcs, &client.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{artifactLabel: artifact.Name})}); err != nil {
		log.FromContext(ctx).Error(err, "failed to list PVCs")
		return ctrl.Result{Requeue: true}, nil
	}

	for _, item := range pvcs.Items {
		pvc = &item
		break
	}

	if pvc == nil {
		log.FromContext(ctx).Error(nil, "failed to locate pvc for artifact, this should not happen")
		return ctrl.Result{}, fmt.Errorf("failed to locate artifact pvc")
	}

	if artifact.Spec.OutputImage != nil {
		idx := fmt.Sprintf("%d", 1)

		job := indexedJobs[idx]
		if job == nil {
			job = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-export-%s", artifact.Name, idx),
					Namespace: artifact.Namespace,
					Annotations: map[string]string{
						artifactExporterIndexAnnotation: idx,
					},
					Labels: map[string]string{
						artifactLabel: artifact.Name,
					},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: ptr(int32(1)),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							InitContainers: []corev1.Container{
								{
									Name:  "init-container",
									Image: "busybox",
									Command: []string{
										"sh", "-c",
										"echo -e 'FROM scratch\nWORKDIR /build\nCOPY *.iso /build' > /artifacts/Dockerfile",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "artifacts",
											MountPath: "/artifacts",
											SubPath:   "artifacts",
										},
									},
								},
							},
						},
					},
				},
			}

			container := corev1.Container{
				Name:  "exporter",
				Image: "gcr.io/kaniko-project/executor:latest",
				Args: []string{
					"--context=/artifacts/",
					"--dockerfile=/artifacts/Dockerfile",
					fmt.Sprintf("--destination=%s/%s:%s", artifact.Spec.OutputImage.Registry, artifact.Spec.OutputImage.Repository, artifact.Spec.OutputImage.Tag),
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "artifacts",
						MountPath: "/artifacts",
						SubPath:   "artifacts",
					},
				},
			}

			if artifact.Spec.OutputImage != nil && artifact.Spec.OutputImage.Cloud == osbuilder.RegistryCloudECR {
				container.Env = []corev1.EnvVar{
					{Name: "AWS_SDK_LOAD_CONFIG", Value: "true"},
					{Name: "AWS_EC2_METADATA_DISABLED", Value: "true"},
				}
			}

			if artifact.Spec.OutputImage != nil && artifact.Spec.OutputImage.DockerConfigSecretKeyRef != nil {
				if err := r.Get(ctx, client.ObjectKey{Namespace: artifact.Namespace, Name: artifact.Spec.OutputImage.DockerConfigSecretKeyRef.Name}, &corev1.Secret{}); err != nil {
					if errors.IsNotFound(err) {
						logger.Info(fmt.Sprintf("Secret %s/%s not found", artifact.Namespace, artifact.Spec.OutputImage.DockerConfigSecretKeyRef.Name))
						return requeue, nil
					}
					return ctrl.Result{}, err
				}
				container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
					Name:      "docker-secret",
					MountPath: "/kaniko/.docker",
				})
				job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
					Name: "docker-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: artifact.Spec.OutputImage.DockerConfigSecretKeyRef.Name,
							Items: []corev1.KeyToPath{{
								Key:  artifact.Spec.OutputImage.DockerConfigSecretKeyRef.Key,
								Path: artifact.Spec.OutputImage.DockerConfigSecretKeyRef.Key,
							}},
						},
					},
				})
			}

			job.Spec.Template.Spec.Containers = append(job.Spec.Template.Spec.Containers, container)

			job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
				Name: "artifacts",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.Name,
						ReadOnly:  false,
					},
				},
			})
			job.Spec.TTLSecondsAfterFinished = ptr(threeHours)
			if err := controllerutil.SetOwnerReference(artifact, job, r.Scheme); err != nil {
				log.FromContext(ctx).Error(err, "failed to set owner reference on job")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Create(ctx, job); err != nil {
				log.FromContext(ctx).Error(err, "failed to create job")
				return ctrl.Result{Requeue: true}, nil
			}

		} else if job.Spec.Completions != nil {
			if job.Status.Succeeded > 0 {
				artifact.Status.Phase = osbuilder.Ready
				if err := TryToUpdateStatus(ctx, r.Client, artifact); err != nil {
					log.FromContext(ctx).Error(err, "failed to update artifact status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			} else if job.Status.Failed > 0 {
				artifact.Status.Phase = osbuilder.Error
				h := getBatchv1JobHealth(job)
				if h.Status == HealthStatusDegraded {
					meta.SetStatusCondition(&artifact.Status.Conditions, metav1.Condition{
						Type:    "Ready",
						Status:  metav1.ConditionFalse,
						Reason:  "Error",
						Message: h.Message,
					})
					if err := TryToUpdateStatus(ctx, r.Client, artifact); err != nil {
						log.FromContext(ctx).Error(err, "failed to update artifact status")
						return ctrl.Result{}, err
					}
					return ctrl.Result{}, nil
				}
			}
		}
	}

	return requeue, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OSArtifactReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&osbuilder.OSArtifact{}).
		Owns(&osbuilder.OSArtifact{}).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.findOwningArtifact),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&batchv1.Job{},
			handler.EnqueueRequestsFromMapFunc(r.findOwningArtifact),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *OSArtifactReconciler) findOwningArtifact(_ context.Context, obj client.Object) []reconcile.Request {
	if obj.GetLabels() == nil {
		return nil
	}

	if artifactName, ok := obj.GetLabels()[artifactLabel]; ok {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name:      artifactName,
					Namespace: obj.GetNamespace(),
				},
			},
		}
	}

	return nil
}
