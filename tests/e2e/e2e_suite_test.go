package e2e_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubectl "github.com/rancher-sandbox/ele-testhelpers/kubectl"
)

var _ = Describe("ISO build test", func() {
	//k := kubectl.New()
	Context("registration", func() {

		AfterEach(func() {
			kubectl.New().Delete("osartifacts", "-n", "default", "hello-c3os")
		})

		It("creates a simple iso", func() {
			err := kubectl.Apply("", "../../tests/fixtures/simple.yaml")
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				b, _ := kubectl.GetData("default", "osartifacts", "hello-c3os", "jsonpath={.spec.imageName}")
				return string(b)
			}, 2*time.Minute, 2*time.Second).Should(Equal("quay.io/c3os/c3os:opensuse-latest"))

			Eventually(func() string {
				b, _ := kubectl.GetData("default", "deployments", "hello-c3os", "jsonpath={.spec.template.metadata.labels.osbuild}")
				return string(b)
			}, 2*time.Minute, 2*time.Second).Should(Equal("workloadhello-c3os"))
			Eventually(func() string {
				b, _ := kubectl.GetData("default", "deployments", "hello-c3os", "jsonpath={.spec.status.unavailableReplicas}")
				return string(b)
			}, 15*time.Minute, 2*time.Second).ShouldNot(Equal("1"))
		})
	})

})
