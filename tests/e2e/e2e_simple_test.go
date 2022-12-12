package e2e_test

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
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

			itHasTheCorrectImage()
			itHasTheCorrectLabels()
			itCopiesTheArtifacts()

			By("deleting the custom resource", func() {
				err = kubectl.New().Delete("osartifacts", "-n", "default", "hello-kairos")
				Expect(err).ToNot(HaveOccurred())
			})

			itCleansUpRoleBindings()
			itDeletesTheArtifacts()
		})
	})
})

func itHasTheCorrectImage() {
	Eventually(func() string {
		b, _ := kubectl.GetData("default", "osartifacts", "hello-kairos", "jsonpath={.spec.imageName}")
		fmt.Printf("looking for image core-opensuse:latest = %+v\n", string(b))
		return string(b)
	}, 2*time.Minute, 2*time.Second).Should(Equal("quay.io/kairos/core-opensuse:latest"))
}

func itHasTheCorrectLabels() {
	Eventually(func() string {
		b, _ := kubectl.GetData("default", "jobs", "hello-kairos", "jsonpath={.spec.template.metadata.labels.osbuild}")
		fmt.Printf("looking for label workloadhello-kairos = %+v\n", string(b))
		return string(b)
	}, 2*time.Minute, 2*time.Second).Should(Equal("workloadhello-kairos"))
}

func itCopiesTheArtifacts() {
	nginxNamespace := "osartifactbuilder-operator-system"
	Eventually(func() string {
		podName := strings.TrimSpace(findPodsWithLabel(nginxNamespace, "app.kubernetes.io/name=osbuilder-nginx"))

		out, _ := kubectl.RunCommandWithOutput(nginxNamespace, podName, "ls /usr/share/nginx/html")

		return out
	}, 15*time.Minute, 2*time.Second).Should(MatchRegexp("hello-kairos.iso"))
}

func itCleansUpRoleBindings() {
	nginxNamespace := "osartifactbuilder-operator-system"
	Eventually(func() string {
		rb := findRoleBindings(nginxNamespace)

		return rb
	}, 3*time.Minute, 2*time.Second).ShouldNot(MatchRegexp("hello-kairos"))
}

func itDeletesTheArtifacts() {
	nginxNamespace := "osartifactbuilder-operator-system"
	Eventually(func() string {
		podName := findPodsWithLabel(nginxNamespace, "app.kubernetes.io/name=osbuilder-nginx")

		out, err := kubectl.RunCommandWithOutput(nginxNamespace, podName, "ls	/usr/share/nginx/html")
		Expect(err).ToNot(HaveOccurred(), out)

		return out
	}, 3*time.Minute, 2*time.Second).ShouldNot(MatchRegexp("hello-kairos.iso"))
}

func findPodsWithLabel(namespace, label string) string {
	kubectlCommand := fmt.Sprintf("kubectl get pods -n %s -l %s --no-headers -o custom-columns=\":metadata.name\" | head -n1", namespace, label)
	cmd := exec.Command("bash", "-c", kubectlCommand)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	Expect(err).ToNot(HaveOccurred(), stderr.String())

	return strings.TrimSpace(out.String())
}

func findRoleBindings(namespace string) string {
	kubectlCommand := fmt.Sprintf("kubectl get rolebindings -n %s --no-headers -o custom-columns=\":metadata.name\"", namespace)
	cmd := exec.Command("bash", "-c", kubectlCommand)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	Expect(err).ToNot(HaveOccurred(), stderr.String())

	return strings.TrimSpace(out.String())
}
