package controllers

import (
	"context"
	"fmt"
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

		r = &OSArtifactReconciler{
			ToolImage: fmt.Sprintf("quay.io/kairos/auroraboot:%s", CompatibleAurorabootVersion),
		}

		// Create a direct client (no cache) for tests - we don't need reconciliation
		// This avoids the complexity of managing a running manager
		directClient, err := client.New(restConfig, client.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred())
		err = r.InjectClient(directClient)
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
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(resp.Object, artifact)
		Expect(err).ToNot(HaveOccurred())
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
		When("BaseImageDockerfile is set", func() {
			BeforeEach(func() {
				secretName := artifact.Name + "-dockerfile"

				_, err := clientset.CoreV1().Secrets(namespace).Create(context.TODO(),
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      secretName,
							Namespace: namespace,
						},
						StringData: map[string]string{
							"Dockerfile": "FROM ubuntu",
						},
						Type: "Opaque",
					}, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				artifact.Spec.BaseImageDockerfile = &osbuilder.SecretKeySelector{
					Name: secretName,
					Key:  "Dockerfile",
				}

				// Whatever, just to let it work
				artifact.Spec.ImageName = "quay.io/kairos-ci/" + artifact.Name + ":latest"
			})

			It("creates an Init Container to build the image", func() {
				pvc, err := r.createPVC(context.TODO(), artifact)
				Expect(err).ToNot(HaveOccurred())

				pod, err := r.createBuilderPod(context.TODO(), artifact, pvc)
				Expect(err).ToNot(HaveOccurred())

				By("checking if an init container was created")
				initContainerNames := []string{}
				for _, c := range pod.Spec.InitContainers {
					initContainerNames = append(initContainerNames, c.Name)
				}
				Expect(initContainerNames).To(ContainElement("kaniko-build"))

				By("checking if init containers complete successfully")
				Eventually(func() bool {
					p, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())

					var allReady = false
					if len(p.Status.InitContainerStatuses) > 0 {
						allReady = true
					}
					for _, c := range p.Status.InitContainerStatuses {
						allReady = allReady && c.Ready
					}

					return allReady
				}, 2*time.Minute, 5*time.Second).Should(BeTrue())

				// req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &v1.PodLogOptions{})
				// podLogs, err := req.Stream(context.TODO())
				// Expect(err).ToNot(HaveOccurred())
				// defer podLogs.Close()

				// buf := new(bytes.Buffer)
				// _, err = io.Copy(buf, podLogs)
				// Expect(err).ToNot(HaveOccurred())
				// str := buf.String()
				// fmt.Printf("str = %+v\n", str)
			})
		})
	})

	Describe("Auroraboot Commands", func() {
		BeforeEach(func() {
			artifact.Spec.ImageName = "quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.6.0"
		})

		When("CloudImage is enabled", func() {
			BeforeEach(func() {
				artifact.Spec.CloudImage = true
			})

			It("creates build-cloud-image container with correct auroraboot command", func() {
				pvc, err := r.createPVC(context.TODO(), artifact)
				Expect(err).ToNot(HaveOccurred())

				pod, err := r.createBuilderPod(context.TODO(), artifact, pvc)
				Expect(err).ToNot(HaveOccurred())

				var cloudImageContainer *corev1.Container
				for i := range pod.Spec.Containers {
					if pod.Spec.Containers[i].Name == "build-cloud-image" {
						cloudImageContainer = &pod.Spec.Containers[i]
						break
					}
				}
				Expect(cloudImageContainer).ToNot(BeNil())
				Expect(cloudImageContainer.Args).To(HaveLen(1))
				Expect(cloudImageContainer.Args[0]).To(ContainSubstring("auroraboot --debug --set 'disk.raw=true'"))
				Expect(cloudImageContainer.Args[0]).To(ContainSubstring("--set 'state_dir=/artifacts'"))
				Expect(cloudImageContainer.Args[0]).To(ContainSubstring("dir:/rootfs"))
				Expect(cloudImageContainer.Args[0]).To(ContainSubstring(fmt.Sprintf("mv /artifacts/*.raw /artifacts/%s.raw", artifact.Name)))
			})

			When("CloudConfigRef is set", func() {
				BeforeEach(func() {
					secretName := artifact.Name + "-cloudconfig"
					_, err := clientset.CoreV1().Secrets(namespace).Create(context.TODO(),
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      secretName,
								Namespace: namespace,
							},
							StringData: map[string]string{
								"cloud-config.yaml": "#cloud-config\nusers:\n  - name: test",
							},
							Type: "Opaque",
						}, metav1.CreateOptions{})
					Expect(err).ToNot(HaveOccurred())

					artifact.Spec.CloudConfigRef = &osbuilder.SecretKeySelector{
						Name: secretName,
						Key:  "cloud-config.yaml",
					}
				})

				It("includes cloud-config flag in auroraboot command", func() {
					pvc, err := r.createPVC(context.TODO(), artifact)
					Expect(err).ToNot(HaveOccurred())

					pod, err := r.createBuilderPod(context.TODO(), artifact, pvc)
					Expect(err).ToNot(HaveOccurred())

					var cloudImageContainer *corev1.Container
					for i := range pod.Spec.Containers {
						if pod.Spec.Containers[i].Name == "build-cloud-image" {
							cloudImageContainer = &pod.Spec.Containers[i]
							break
						}
					}
					Expect(cloudImageContainer).ToNot(BeNil())
					Expect(cloudImageContainer.Args[0]).To(ContainSubstring("--cloud-config /cloud-config.yaml"))
				})
			})
		})

		When("Netboot is enabled", func() {
			BeforeEach(func() {
				artifact.Spec.Netboot = true
				artifact.Spec.ISO = true
				artifact.Spec.NetbootURL = "http://example.com"
			})

			It("creates build-netboot container with correct auroraboot netboot command", func() {
				pvc, err := r.createPVC(context.TODO(), artifact)
				Expect(err).ToNot(HaveOccurred())

				pod, err := r.createBuilderPod(context.TODO(), artifact, pvc)
				Expect(err).ToNot(HaveOccurred())

				var netbootContainer *corev1.Container
				for i := range pod.Spec.Containers {
					if pod.Spec.Containers[i].Name == "build-netboot" {
						netbootContainer = &pod.Spec.Containers[i]
						break
					}
				}
				Expect(netbootContainer).ToNot(BeNil())
				Expect(netbootContainer.Args).To(HaveLen(1))
				Expect(netbootContainer.Args[0]).To(ContainSubstring("auroraboot --debug netboot"))
				Expect(netbootContainer.Args[0]).To(ContainSubstring(fmt.Sprintf("/artifacts/%s.iso", artifact.Name)))
				Expect(netbootContainer.Args[0]).To(ContainSubstring("/artifacts"))
				Expect(netbootContainer.Args[0]).To(ContainSubstring(artifact.Name))
			})
		})

		When("AzureImage is enabled", func() {
			BeforeEach(func() {
				artifact.Spec.AzureImage = true
			})

			It("creates build-azure-cloud-image container with correct auroraboot command", func() {
				pvc, err := r.createPVC(context.TODO(), artifact)
				Expect(err).ToNot(HaveOccurred())

				pod, err := r.createBuilderPod(context.TODO(), artifact, pvc)
				Expect(err).ToNot(HaveOccurred())

				var azureContainer *corev1.Container
				for i := range pod.Spec.Containers {
					if pod.Spec.Containers[i].Name == "build-azure-cloud-image" {
						azureContainer = &pod.Spec.Containers[i]
						break
					}
				}
				Expect(azureContainer).ToNot(BeNil())
				Expect(azureContainer.Args).To(HaveLen(1))
				Expect(azureContainer.Args[0]).To(ContainSubstring("auroraboot --debug --set 'disk.vhd=true'"))
				Expect(azureContainer.Args[0]).To(ContainSubstring("--set 'state_dir=/artifacts'"))
				Expect(azureContainer.Args[0]).To(ContainSubstring("dir:/rootfs"))
				Expect(azureContainer.Args[0]).To(ContainSubstring(fmt.Sprintf("mv /artifacts/*.vhd /artifacts/%s.vhd", artifact.Name)))
			})

			When("CloudConfigRef is set", func() {
				BeforeEach(func() {
					secretName := artifact.Name + "-cloudconfig"
					_, err := clientset.CoreV1().Secrets(namespace).Create(context.TODO(),
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      secretName,
								Namespace: namespace,
							},
							StringData: map[string]string{
								"cloud-config.yaml": "#cloud-config\nusers:\n  - name: test",
							},
							Type: "Opaque",
						}, metav1.CreateOptions{})
					Expect(err).ToNot(HaveOccurred())

					artifact.Spec.CloudConfigRef = &osbuilder.SecretKeySelector{
						Name: secretName,
						Key:  "cloud-config.yaml",
					}
				})

				It("includes cloud-config flag in auroraboot command", func() {
					pvc, err := r.createPVC(context.TODO(), artifact)
					Expect(err).ToNot(HaveOccurred())

					pod, err := r.createBuilderPod(context.TODO(), artifact, pvc)
					Expect(err).ToNot(HaveOccurred())

					var azureContainer *corev1.Container
					for i := range pod.Spec.Containers {
						if pod.Spec.Containers[i].Name == "build-azure-cloud-image" {
							azureContainer = &pod.Spec.Containers[i]
							break
						}
					}
					Expect(azureContainer).ToNot(BeNil())
					Expect(azureContainer.Args[0]).To(ContainSubstring("--cloud-config /cloud-config.yaml"))
				})
			})
		})

		When("GCEImage is enabled", func() {
			BeforeEach(func() {
				artifact.Spec.GCEImage = true
			})

			It("creates build-gce-cloud-image container with correct auroraboot command", func() {
				pvc, err := r.createPVC(context.TODO(), artifact)
				Expect(err).ToNot(HaveOccurred())

				pod, err := r.createBuilderPod(context.TODO(), artifact, pvc)
				Expect(err).ToNot(HaveOccurred())

				var gceContainer *corev1.Container
				for i := range pod.Spec.Containers {
					if pod.Spec.Containers[i].Name == "build-gce-cloud-image" {
						gceContainer = &pod.Spec.Containers[i]
						break
					}
				}
				Expect(gceContainer).ToNot(BeNil())
				Expect(gceContainer.Args).To(HaveLen(1))
				Expect(gceContainer.Args[0]).To(ContainSubstring("auroraboot --debug --set 'disk.gce=true'"))
				Expect(gceContainer.Args[0]).To(ContainSubstring("--set 'state_dir=/artifacts'"))
				Expect(gceContainer.Args[0]).To(ContainSubstring("dir:/rootfs"))
				Expect(gceContainer.Args[0]).To(ContainSubstring(fmt.Sprintf("mv /artifacts/*.raw.gce.tar.gz /artifacts/%s.gce.tar.gz", artifact.Name)))
			})

			When("CloudConfigRef is set", func() {
				BeforeEach(func() {
					secretName := artifact.Name + "-cloudconfig"
					_, err := clientset.CoreV1().Secrets(namespace).Create(context.TODO(),
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      secretName,
								Namespace: namespace,
							},
							StringData: map[string]string{
								"cloud-config.yaml": "#cloud-config\nusers:\n  - name: test",
							},
							Type: "Opaque",
						}, metav1.CreateOptions{})
					Expect(err).ToNot(HaveOccurred())

					artifact.Spec.CloudConfigRef = &osbuilder.SecretKeySelector{
						Name: secretName,
						Key:  "cloud-config.yaml",
					}
				})

				It("includes cloud-config flag in auroraboot command", func() {
					pvc, err := r.createPVC(context.TODO(), artifact)
					Expect(err).ToNot(HaveOccurred())

					pod, err := r.createBuilderPod(context.TODO(), artifact, pvc)
					Expect(err).ToNot(HaveOccurred())

					var gceContainer *corev1.Container
					for i := range pod.Spec.Containers {
						if pod.Spec.Containers[i].Name == "build-gce-cloud-image" {
							gceContainer = &pod.Spec.Containers[i]
							break
						}
					}
					Expect(gceContainer).ToNot(BeNil())
					Expect(gceContainer.Args[0]).To(ContainSubstring("--cloud-config /cloud-config.yaml"))
				})
			})
		})
	})
})
