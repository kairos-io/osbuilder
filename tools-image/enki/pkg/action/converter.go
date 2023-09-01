package action

import (
	"fmt"
	"os"
	"os/exec"
)

// ConverterAction is the action that converts a non-kairos image to a Kairos one.
// The conversion happens in a best-effort manner. It's not guaranteed that
// any distribution will successfully be converted to a Kairos flavor. See
// the Kairos releases for known-to-work flavors.
// The "input" of this action is a directory where the rootfs is extracted.
// [TBD] The output is the same directory updated to be a Kairos image
type ConverterAction struct {
	rootFSPath string
}

func NewConverterAction(rootfsPath string) *ConverterAction {
	return &ConverterAction{
		rootFSPath: rootfsPath,
	}
}

// https://github.com/GoogleContainerTools/kaniko/issues/1007
// docker run -it -v $PWD:/work:rw  --rm gcr.io/kaniko-project/executor:latest --dockerfile /work/Dockerfile --context dir:///work/rootfs --destination whatever --tar-path /work/image.tar --no-push

// Run assumes the `kaniko` executable is in PATH as it shells out to it.
// The best way to do that is to spin up a container with the upstream
// image (gcr.io/kaniko-project/executor:latest) and mount enki in it.
// E.g.
// docker run -it -e PATH=/kaniko -v /tmp -v /home/dimitris/workspace/kairos/osbuilder/tmp/:/context -v "$PWD/build/enki":/enki --rm --entrypoint "/enki" gcr.io/kaniko-project/executor:latest convert /context
func (ca *ConverterAction) Run() (err error) {
	dockerfile, err := ca.createDockerfile()
	if err != nil {
		return
	}
	defer os.Remove(dockerfile)

	err = ca.addDockerIgnore()
	if err != nil {
		return
	}
	defer ca.removeDockerIgnore()

	out, err := ca.BuildWithKaniko(dockerfile)
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	fmt.Printf("out = %+v\n", out)

	return
}

func (ca *ConverterAction) createDockerfile() (string, error) {
	f, err := os.CreateTemp("", "")
	if err != nil {
		return "", err
	}
	defer f.Close()

	// write data to the temporary file
	data := []byte(`
FROM scratch as rootfs
COPY . .

FROM rootfs

# TODO: Do more clever things
RUN apt-get install -y curl
`)

	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

// https://github.com/GoogleContainerTools/kaniko/issues/1007
// Create a .dockerignore in the rootfs directory to skip these:
// https://github.com/GoogleContainerTools/kaniko/pull/1724/files#diff-1e90758e2fb0f26bdbfe7a40aafc4b4796cbf808842703e52e16c1f36b8da7dcR89
func (ca *ConverterAction) addDockerIgnore() error {
	return nil
}

func (ca *ConverterAction) removeDockerIgnore() error {
	return nil
}

func (ca *ConverterAction) BuildWithKaniko(dockerfile string) (string, error) {
	fmt.Printf("ca.rootFSPath = %+v\n", ca.rootFSPath)
	cmd := exec.Command("executor",
		"--dockerfile", dockerfile,
		"--context", ca.rootFSPath,
		"--destination", "whatever",
		"--tar-path", "image.tar", // TODO: Where do we write? Do we want this extracted to the rootFSPath?
		"--no-push",
	)

	d, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, string(d))
	}

	return string(d), err
}
