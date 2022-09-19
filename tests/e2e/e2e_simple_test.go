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
			kubectl.New().Delete("osartifacts", "-n", "default", "hello-kairos")
		})

		It("creates a simple iso", func() {
			err := kubectl.Apply("", "../../tests/fixtures/simple.yaml")
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				b, _ := kubectl.GetData("default", "osartifacts", "hello-kairos", "jsonpath={.spec.imageName}")
				return string(b)
			}, 2*time.Minute, 2*time.Second).Should(Equal("quay.io/kairos/core-opensuse:latest"))

			Eventually(func() string {
				b, _ := kubectl.GetData("default", "deployments", "hello-kairos", "jsonpath={.spec.template.metadata.labels.osbuild}")
				return string(b)
			}, 2*time.Minute, 2*time.Second).Should(Equal("workloadhello-kairos"))
			Eventually(func() string {
				b, _ := kubectl.GetData("default", "deployments", "hello-kairos", "jsonpath={.spec.status.unavailableReplicas}")
				return string(b)
			}, 15*time.Minute, 2*time.Second).ShouldNot(Equal("1"))
		})
	})

})
