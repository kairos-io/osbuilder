package e2e_test

import (
	"context"
	"testing"
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

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "kairos-operator e2e test Suite")
}

// TestClients holds common Kubernetes clients used across e2e tests
type TestClients struct {
	Artifacts dynamic.ResourceInterface
	Pods      dynamic.ResourceInterface
	PVCs      dynamic.ResourceInterface
	Jobs      dynamic.ResourceInterface
	Scheme    *runtime.Scheme
}

// SetupTestClients initializes and returns common Kubernetes clients
func SetupTestClients() *TestClients {
	k8s := dynamic.NewForConfigOrDie(ctrl.GetConfigOrDie())
	scheme := runtime.NewScheme()
	err := osbuilder.AddToScheme(scheme)
	Expect(err).ToNot(HaveOccurred())

	return &TestClients{
		Artifacts: k8s.Resource(schema.GroupVersionResource{
			Group:    osbuilder.GroupVersion.Group,
			Version:  osbuilder.GroupVersion.Version,
			Resource: "osartifacts",
		}).Namespace("default"),
		Pods: k8s.Resource(schema.GroupVersionResource{
			Group:    corev1.GroupName,
			Version:  corev1.SchemeGroupVersion.Version,
			Resource: "pods",
		}).Namespace("default"),
		PVCs: k8s.Resource(schema.GroupVersionResource{
			Group:    corev1.GroupName,
			Version:  corev1.SchemeGroupVersion.Version,
			Resource: "persistentvolumeclaims",
		}).Namespace("default"),
		Jobs: k8s.Resource(schema.GroupVersionResource{
			Group:    batchv1.GroupName,
			Version:  batchv1.SchemeGroupVersion.Version,
			Resource: "jobs",
		}).Namespace("default"),
		Scheme: scheme,
	}
}

// CreateArtifact creates an OSArtifact and returns its name and label selector
func (tc *TestClients) CreateArtifact(artifact *osbuilder.OSArtifact) (string, labels.Selector) {
	uArtifact := unstructured.Unstructured{}
	uArtifact.Object, _ = runtime.DefaultUnstructuredConverter.ToUnstructured(artifact)
	resp, err := tc.Artifacts.Create(context.TODO(), &uArtifact, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	artifactName := resp.GetName()

	artifactLabelSelectorReq, err := labels.NewRequirement("build.kairos.io/artifact", selection.Equals, []string{artifactName})
	Expect(err).ToNot(HaveOccurred())
	artifactLabelSelector := labels.NewSelector().Add(*artifactLabelSelectorReq)

	return artifactName, artifactLabelSelector
}

// WaitForBuildCompletion waits for the build pod to complete and artifact to be ready
func (tc *TestClients) WaitForBuildCompletion(artifactName string, artifactLabelSelector labels.Selector) {
	By("waiting for build pod to complete")
	Eventually(func(g Gomega) {
		w, err := tc.Pods.Watch(context.TODO(), metav1.ListOptions{LabelSelector: artifactLabelSelector.String()})
		g.Expect(err).ToNot(HaveOccurred())

		var stopped bool
		for !stopped {
			event, ok := <-w.ResultChan()
			stopped = event.Type != watch.Deleted && event.Type != watch.Error || !ok
		}
	}).WithTimeout(time.Hour).Should(Succeed())

	By("waiting for artifact to be ready")
	Eventually(func(g Gomega) {
		w, err := tc.Artifacts.Watch(context.TODO(), metav1.ListOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		var artifact osbuilder.OSArtifact
		var stopped bool
		for !stopped {
			event, ok := <-w.ResultChan()
			stopped = !ok

			if event.Type == watch.Modified && event.Object.(*unstructured.Unstructured).GetName() == artifactName {
				err := tc.Scheme.Convert(event.Object, &artifact, nil)
				g.Expect(err).ToNot(HaveOccurred())
				stopped = artifact.Status.Phase == osbuilder.Ready
			}
		}
	}).WithTimeout(time.Hour).Should(Succeed())
}

// WaitForExportCompletion waits for the export job to complete
func (tc *TestClients) WaitForExportCompletion(artifactLabelSelector labels.Selector) {
	By("waiting for export job to complete")
	Eventually(func(g Gomega) {
		w, err := tc.Jobs.Watch(context.TODO(), metav1.ListOptions{LabelSelector: artifactLabelSelector.String()})
		g.Expect(err).ToNot(HaveOccurred())

		var stopped bool
		for !stopped {
			event, ok := <-w.ResultChan()
			stopped = event.Type != watch.Deleted && event.Type != watch.Error || !ok
		}
	}).WithTimeout(time.Hour).Should(Succeed())
}

// Cleanup deletes the artifact and waits for all related resources to be cleaned up
func (tc *TestClients) Cleanup(artifactName string, artifactLabelSelector labels.Selector) {
	By("cleaning up resources")
	err := tc.Artifacts.Delete(context.TODO(), artifactName, metav1.DeleteOptions{})
	Expect(err).ToNot(HaveOccurred())

	Eventually(func(g Gomega) int {
		res, err := tc.Artifacts.List(context.TODO(), metav1.ListOptions{})
		g.Expect(err).ToNot(HaveOccurred())
		return len(res.Items)
	}).WithTimeout(time.Minute).Should(Equal(0))
	Eventually(func(g Gomega) int {
		res, err := tc.Pods.List(context.TODO(), metav1.ListOptions{LabelSelector: artifactLabelSelector.String()})
		g.Expect(err).ToNot(HaveOccurred())
		return len(res.Items)
	}).WithTimeout(time.Minute).Should(Equal(0))
	Eventually(func(g Gomega) int {
		res, err := tc.PVCs.List(context.TODO(), metav1.ListOptions{LabelSelector: artifactLabelSelector.String()})
		g.Expect(err).ToNot(HaveOccurred())
		return len(res.Items)
	}).WithTimeout(time.Minute).Should(Equal(0))
	Eventually(func(g Gomega) int {
		res, err := tc.Jobs.List(context.TODO(), metav1.ListOptions{LabelSelector: artifactLabelSelector.String()})
		g.Expect(err).ToNot(HaveOccurred())
		return len(res.Items)
	}).WithTimeout(time.Minute).Should(Equal(0))
}
