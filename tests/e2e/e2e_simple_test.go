package e2e_test

import (
	osbuilder "github.com/kairos-io/osbuilder/api/v1alpha2"
	. "github.com/onsi/ginkgo/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("ISO build test", func() {
	var artifactName string
	var artifactLabelSelector labels.Selector
	var tc *TestClients

	BeforeEach(func() {
		tc = SetupTestClients()

		artifact := &osbuilder.OSArtifact{
			TypeMeta: metav1.TypeMeta{
				Kind:       "OSArtifact",
				APIVersion: osbuilder.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "simple-",
			},
			Spec: osbuilder.OSArtifactSpec{
				ImageName: "quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.6.0",
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
										Args:    []string{"-xec", "[ -f /artifacts/*.iso ]"},
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

		artifactName, artifactLabelSelector = tc.CreateArtifact(artifact)
	})

	It("works", func() {
		tc.WaitForBuildCompletion(artifactName, artifactLabelSelector)
		tc.WaitForExportCompletion(artifactLabelSelector)
		tc.Cleanup(artifactName, artifactLabelSelector)
	})
})
