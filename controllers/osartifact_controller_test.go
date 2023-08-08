package controllers

import (
	"context"

	osbuilder "github.com/kairos-io/osbuilder/api/v1alpha2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("OSArtifactReconciler", func() {
	var r *OSArtifactReconciler
	var artifact *osbuilder.OSArtifact
	var namespace string
	var restConfig *rest.Config
	var clientset *kubernetes.Clientset
	var err error

	BeforeEach(func() {
		restConfig = ctrl.GetConfigOrDie()
		clientset, err = kubernetes.NewForConfig(restConfig)
		Expect(err).ToNot(HaveOccurred())
		namespace = createRandomNamespace(clientset)

		artifact = &osbuilder.OSArtifact{
			TypeMeta: metav1.TypeMeta{
				Kind:       "OSArtifact",
				APIVersion: osbuilder.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      randStringRunes(10),
			},
		}

		scheme := runtime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		utilruntime.Must(osbuilder.AddToScheme(scheme))

		mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
			Scheme: scheme,
		})
		Expect(err).ToNot(HaveOccurred())

		r = &OSArtifactReconciler{}
		err = (r).SetupWithManager(mgr)
		Expect(err).ToNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		k8s := dynamic.NewForConfigOrDie(restConfig)
		artifacts := k8s.Resource(
			schema.GroupVersionResource{
				Group:    osbuilder.GroupVersion.Group,
				Version:  osbuilder.GroupVersion.Version,
				Resource: "osartifacts"}).Namespace(namespace)

		uArtifact := unstructured.Unstructured{}
		uArtifact.Object, _ = runtime.DefaultUnstructuredConverter.ToUnstructured(artifact)
		resp, err := artifacts.Create(context.TODO(), &uArtifact, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		// Update the local object with the one fetched from k8s
		runtime.DefaultUnstructuredConverter.FromUnstructured(resp.Object, artifact)
	})

	AfterEach(func() {
		deleteNamepace(clientset, namespace)
	})

	Describe("CreateConfigMap", func() {
		It("creates a ConfigMap with no error", func() {
			ctx := context.Background()
			err := r.CreateConfigMap(ctx, artifact)
			Expect(err).ToNot(HaveOccurred())
			c, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), artifact.Name, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(c).ToNot(BeNil())
		})
	})

	Describe("CreateBuilderPod", func() {
		BeforeEach(func() {
			c, err := clientset.CoreV1().Secrets(namespace).Create(context.TODO(),
				&corev1.Secret{},
				artifact.Name+"-dockerfile", metav1.CreateOptions{})

			artifact.Spec.BaseImageDockerfile := osbuilder.SecretKeySelector{
				Name: "",
				Key:  "",
			}

		})
		// TODO: Add a dockerfile to the artifact and check that the image was built
		It("creates an initcontainer to build the image", func() {
		})
	})
})
