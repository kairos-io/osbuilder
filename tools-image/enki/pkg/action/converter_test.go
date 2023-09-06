package action_test

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"time"

	. "github.com/kairos-io/enki/pkg/action"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("ConverterAction", func() {
	var rootfsPath, resultDir, imageName string
	var action *ConverterAction

	BeforeEach(func() {
		rootfsPath = prepareRootfs()
		resultDir = prepareResultDir()
		imageName = newImageName(10)
		action = NewConverterAction(rootfsPath, path.Join(resultDir, "image.tar"), imageName)
	})

	AfterEach(func() {
		cleanupDir(rootfsPath)
		cleanupDir(resultDir)
		removeImage(imageName)
	})

	It("adds the framework bits", func() {
		// TODO: Run enki next to kaniko (in an image?)
		// CGO_ENABLED=0 go build -ldflags '-extldflags "-static"' -o build/enki && docker run -it -e PATH=/kaniko -v /tmp -v /home/dimitris/workspace/kairos/osbuilder/tmp/rootfs/:/context -v "$PWD/build/enki":/enki -v $PWD:/build --rm --entrypoint "/enki" gcr.io/kaniko-project/executor:latest convert /context
		Expect(action.Run()).ToNot(HaveOccurred())

		cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cat %s/image.tar | docker load", resultDir))
		out, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), string(out))
	})
})

func prepareRootfs() string {
	dir, err := os.MkdirTemp("", "kairos-temp")
	Expect(err).ToNot(HaveOccurred())

	cmd := exec.Command("/bin/sh", "-c",
		fmt.Sprintf("docker run -v %s:/work quay.io/luet/base util unpack ubuntu:latest /work", dir))
	out, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), string(out))

	return dir
}

func prepareResultDir() string {
	dir, err := os.MkdirTemp("", "kairos-temp")
	Expect(err).ToNot(HaveOccurred())

	return dir
}

func newImageName(n int) string {
	rand.Seed(time.Now().UnixNano())
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func removeImage(image string) {
	fmt.Printf("image = %+v\n", image)
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("docker rmi %s:latest", image))
	out, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), string(out))
}

// Cleanup in docker to use the same permissions as those when we created.
// This way we avoid sudo.
func cleanupDir(path string) {
	fmt.Printf("path = %+v\n", path)
	cmd := exec.Command("/bin/sh", "-c",
		fmt.Sprintf("docker run --rm -v %[1]s:/work ubuntu /bin/bash -c 'rm -rf /work/*'", path))
	out, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), string(out))
	Expect(os.RemoveAll(path)).ToNot(HaveOccurred())
}
