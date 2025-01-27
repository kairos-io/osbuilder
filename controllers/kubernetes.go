package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/client-go/util/retry"
	"k8s.io/kubectl/pkg/util/podutils"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TryToUpdateStatus(ctx context.Context, client ctrlruntimeclient.Client, object ctrlruntimeclient.Object) error {
	key := ctrlruntimeclient.ObjectKeyFromObject(object)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		original := object.DeepCopyObject().(ctrlruntimeclient.Object)
		if err := client.Get(ctx, key, object); err != nil {
			return fmt.Errorf("could not fetch current %s/%s state, got error: %+v", object.GetName(), object.GetNamespace(), err)
		}

		if reflect.DeepEqual(object, original) {
			return nil
		}

		return client.Status().Patch(ctx, original, ctrlruntimeclient.MergeFrom(object))
	})

}

const (
	// Indicates that health assessment failed and actual health status is unknown
	HealthStatusUnknown HealthStatusCode = "Unknown"
	// Progressing health status means that resource is not healthy but still have a chance to reach healthy state
	HealthStatusProgressing HealthStatusCode = "Progressing"
	// Resource is 100% healthy
	HealthStatusHealthy HealthStatusCode = "Healthy"
	// Assigned to resources that are suspended or paused. The typical example is a
	// [suspended](https://kubernetes.io/docs/tasks/job/automated-tasks-with-cron-jobs/#suspend) CronJob.
	HealthStatusSuspended HealthStatusCode = "Suspended"
	HealthStatusPaused    HealthStatusCode = "Paused"
	// Degrade status is used if resource status indicates failure or resource could not reach healthy state
	// within some timeout.
	HealthStatusDegraded HealthStatusCode = "Degraded"
	// Indicates that resource is missing in the cluster.
	HealthStatusMissing HealthStatusCode = "Missing"
)

// Represents resource health status
type HealthStatusCode string

type HealthStatus struct {
	Status  HealthStatusCode `json:"status,omitempty"`
	Message string           `json:"message,omitempty"`
}

func getCorev1PodHealth(pod *corev1.Pod) *HealthStatus {
	// This logic cannot be applied when the pod.Spec.RestartPolicy is: corev1.RestartPolicyOnFailure,
	// corev1.RestartPolicyNever, otherwise it breaks the resource hook logic.
	// The issue is, if we mark a pod with ImagePullBackOff as Degraded, and the pod is used as a resource hook,
	// then we will prematurely fail the PreSync/PostSync hook. Meanwhile, when that error condition is resolved
	// (e.g. the image is available), the resource hook pod will unexpectedly be executed even though the sync has
	// completed.
	if pod.Spec.RestartPolicy == corev1.RestartPolicyAlways {
		var status HealthStatusCode
		var messages []string

		for _, containerStatus := range pod.Status.ContainerStatuses {
			waiting := containerStatus.State.Waiting
			// Article listing common container errors: https://medium.com/kokster/debugging-crashloopbackoffs-with-init-containers-26f79e9fb5bf
			if waiting != nil && (strings.HasPrefix(waiting.Reason, "Err") || strings.HasSuffix(waiting.Reason, "Error") || strings.HasSuffix(waiting.Reason, "BackOff")) {
				status = HealthStatusDegraded
				messages = append(messages, waiting.Message)
			}
		}

		if status != "" {
			return &HealthStatus{
				Status:  status,
				Message: strings.Join(messages, ", "),
			}
		}
	}

	getFailMessage := func(ctr *corev1.ContainerStatus) string {
		if ctr.State.Terminated != nil {
			if ctr.State.Terminated.Message != "" {
				return ctr.State.Terminated.Message
			}
			if ctr.State.Terminated.Reason == "OOMKilled" {
				return ctr.State.Terminated.Reason
			}
			if ctr.State.Terminated.ExitCode != 0 {
				return fmt.Sprintf("container %q failed with exit code %d", ctr.Name, ctr.State.Terminated.ExitCode)
			}
		}
		return ""
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		return &HealthStatus{
			Status:  HealthStatusProgressing,
			Message: pod.Status.Message,
		}
	case corev1.PodSucceeded:
		return &HealthStatus{
			Status:  HealthStatusHealthy,
			Message: pod.Status.Message,
		}
	case corev1.PodFailed:
		if pod.Status.Message != "" {
			// Pod has a nice error message. Use that.
			return &HealthStatus{Status: HealthStatusDegraded, Message: pod.Status.Message}
		}
		for _, ctr := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			if msg := getFailMessage(&ctr); msg != "" {
				return &HealthStatus{Status: HealthStatusDegraded, Message: msg}
			}
		}

		return &HealthStatus{Status: HealthStatusDegraded, Message: ""}
	case corev1.PodRunning:
		switch pod.Spec.RestartPolicy {
		case corev1.RestartPolicyAlways:
			// if pod is ready, it is automatically healthy
			if podutils.IsPodReady(pod) {
				return &HealthStatus{
					Status:  HealthStatusHealthy,
					Message: pod.Status.Message,
				}
			}
			// if it's not ready, check to see if any container terminated, if so, it's degraded
			for _, ctrStatus := range pod.Status.ContainerStatuses {
				if ctrStatus.LastTerminationState.Terminated != nil {
					return &HealthStatus{
						Status:  HealthStatusDegraded,
						Message: pod.Status.Message,
					}
				}
			}
			// otherwise we are progressing towards a ready state
			return &HealthStatus{
				Status:  HealthStatusProgressing,
				Message: pod.Status.Message,
			}
		case corev1.RestartPolicyOnFailure, corev1.RestartPolicyNever:
			// pods set with a restart policy of OnFailure or Never, have a finite life.
			// These pods are typically resource hooks. Thus, we consider these as Progressing
			// instead of healthy.
			return &HealthStatus{
				Status:  HealthStatusProgressing,
				Message: pod.Status.Message,
			}
		}
	}
	return &HealthStatus{
		Status:  HealthStatusUnknown,
		Message: pod.Status.Message,
	}
}

func getBatchv1JobHealth(job *batchv1.Job) *HealthStatus {
	failed := false
	var failMsg string
	complete := false
	var message string
	isSuspended := false
	for _, condition := range job.Status.Conditions {
		switch condition.Type {
		case batchv1.JobFailed:
			failed = true
			complete = true
			failMsg = condition.Message
		case batchv1.JobComplete:
			complete = true
			message = condition.Message
		case batchv1.JobSuspended:
			complete = true
			message = condition.Message
			if condition.Status == corev1.ConditionTrue {
				isSuspended = true
			}
		}
	}

	if !complete {
		return &HealthStatus{
			Status:  HealthStatusProgressing,
			Message: message,
		}
	}
	if failed {
		return &HealthStatus{
			Status:  HealthStatusDegraded,
			Message: failMsg,
		}
	}
	if isSuspended {
		return &HealthStatus{
			Status:  HealthStatusSuspended,
			Message: failMsg,
		}
	}

	return &HealthStatus{
		Status:  HealthStatusHealthy,
		Message: message,
	}

}
