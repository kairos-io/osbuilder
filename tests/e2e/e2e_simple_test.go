package e2e_test

import (
	"context"
	"time"

	osbuilder "github.com/kairos-io/osbuilder/api/v1alpha2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("ISO build test", func() {
	var artifactName string
	var artifacts, pods, pvcs, jobs dynamic.ResourceInterface
	var scheme *runtime.Scheme
	var artifactLabelSelector labels.Selector

	BeforeEach(func() {
		k8s := dynamic.NewForConfigOrDie(ctrl.GetConfigOrDie())
		scheme = runtime.NewScheme()
		err := osbuilder.AddToScheme(scheme)
		Expect(err).ToNot(HaveOccurred())

		artifacts = k8s.Resource(schema.GroupVersionResource{Group: osbuilder.GroupVersion.Group, Version: osbuilder.GroupVersion.Version, Resource: "osartifacts"}).Namespace("default")
		pods = k8s.Resource(schema.GroupVersionResource{Group: corev1.GroupName, Version: corev1.SchemeGroupVersion.Version, Resource: "pods"}).Namespace("default")
		pvcs = k8s.Resource(schema.GroupVersionResource{Group: corev1.GroupName, Version: corev1.SchemeGroupVersion.Version, Resource: "persistentvolumeclaims"}).Namespace("default")
		jobs = k8s.Resource(schema.GroupVersionResource{Group: batchv1.GroupName, Version: batchv1.SchemeGroupVersion.Version, Resource: "jobs"}).Namespace("default")

		artifact := &osbuilder.OSArtifact{
			TypeMeta: metav1.TypeMeta{
				Kind:       "OSArtifact",
				APIVersion: osbuilder.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "simple-",
			},
			Spec: osbuilder.OSArtifactSpec{
				ImageName: "quay.io/kairos/core-opensuse:latest",
				ISO:       true,
				DiskSize:  "",
				Exporters: []batchv1.JobSpec{
					{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyNever,
								Containers: []corev1.Container{
									{
										Name:    "test",
										Image:   "debian:latest",
										Command: []string{"bash"},
										Args:    []string{"-xec", "[ -f /artifacts/build/*.iso ]"},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "artifacts",
												ReadOnly:  true,
												MountPath: "/artifacts",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		uArtifact := unstructured.Unstructured{}
		uArtifact.Object, _ = runtime.DefaultUnstructuredConverter.ToUnstructured(artifact)
		resp, err := artifacts.Create(context.TODO(), &uArtifact, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		artifactName = resp.GetName()

		artifactLabelSelectorReq, err := labels.NewRequirement("build.kairos.io/artifact", selection.Equals, []string{artifactName})
		Expect(err).ToNot(HaveOccurred())
		artifactLabelSelector = labels.NewSelector().Add(*artifactLabelSelectorReq)
	})

	It("works", func() {
		By("starting the build")
		Eventually(func(g Gomega) {
			w, err := pods.Watch(context.TODO(), metav1.ListOptions{LabelSelector: artifactLabelSelector.String()})
			Expect(err).ToNot(HaveOccurred())

			var stopped bool
			for !stopped {
				event, ok := <-w.ResultChan()

				stopped = event.Type != watch.Deleted && event.Type != watch.Error || !ok
			}
		}).WithTimeout(time.Hour).Should(Succeed())

		By("exporting the artifacts")
		Eventually(func(g Gomega) {
			w, err := jobs.Watch(context.TODO(), metav1.ListOptions{LabelSelector: artifactLabelSelector.String()})
			Expect(err).ToNot(HaveOccurred())

			var stopped bool
			for !stopped {
				event, ok := <-w.ResultChan()

				stopped = event.Type != watch.Deleted && event.Type != watch.Error || !ok
			}
		}).WithTimeout(time.Hour).Should(Succeed())

		By("building the artifacts successfully")
		Eventually(func(g Gomega) {
			w, err := artifacts.Watch(context.TODO(), metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())

			var artifact osbuilder.OSArtifact
			var stopped bool
			for !stopped {
				event, ok := <-w.ResultChan()
				stopped = !ok

				if event.Type == watch.Modified && event.Object.(*unstructured.Unstructured).GetName() == artifactName {
					err := scheme.Convert(event.Object, &artifact, nil)
					Expect(err).ToNot(HaveOccurred())
					stopped = artifact.Status.Phase == osbuilder.Ready
				}

			}
		}).WithTimeout(time.Hour).Should(Succeed())

		By("cleaning up resources on deletion")
		err := artifacts.Delete(context.TODO(), artifactName, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func(g Gomega) int {
			res, err := artifacts.List(context.TODO(), metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())
			return len(res.Items)
		}).WithTimeout(time.Minute).Should(Equal(0))
		Eventually(func(g Gomega) int {
			res, err := pods.List(context.TODO(), metav1.ListOptions{LabelSelector: artifactLabelSelector.String()})
			Expect(err).ToNot(HaveOccurred())
			return len(res.Items)
		}).WithTimeout(time.Minute).Should(Equal(0))
		Eventually(func(g Gomega) int {
			res, err := pvcs.List(context.TODO(), metav1.ListOptions{LabelSelector: artifactLabelSelector.String()})
			Expect(err).ToNot(HaveOccurred())
			return len(res.Items)
		}).WithTimeout(time.Minute).Should(Equal(0))
		Eventually(func(g Gomega) int {
			res, err := jobs.List(context.TODO(), metav1.ListOptions{LabelSelector: artifactLabelSelector.String()})
			Expect(err).ToNot(HaveOccurred())
			return len(res.Items)
		}).WithTimeout(time.Minute).Should(Equal(0))
	})
})
