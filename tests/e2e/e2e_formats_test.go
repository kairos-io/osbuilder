package e2e_test

import (
	osbuilder "github.com/kairos-io/osbuilder/api/v1alpha2"
	. "github.com/onsi/ginkgo/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("Artifact Format Tests", func() {
	var tc *TestClients

	BeforeEach(func() {
		tc = SetupTestClients()
	})

	Describe("CloudImage (Raw Disk)", func() {
		var artifactName string
		var artifactLabelSelector labels.Selector

		BeforeEach(func() {
			artifact := &osbuilder.OSArtifact{
				TypeMeta: metav1.TypeMeta{
					Kind:       "OSArtifact",
					APIVersion: osbuilder.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "cloudimage-",
				},
				Spec: osbuilder.OSArtifactSpec{
					ImageName:  "quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.6.0",
					CloudImage: true,
					Exporters: []batchv1.JobSpec{
						{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									RestartPolicy: corev1.RestartPolicyNever,
									Containers: []corev1.Container{
										{
											Name:    "verify",
											Image:   "debian:latest",
											Command: []string{"bash"},
											Args: []string{
												"-xec",
												`
													set -e
													# Check that raw file exists
													raw_file=$(ls /artifacts/*.raw 2>/dev/null | head -n1)
													if [ -z "$raw_file" ]; then
														echo "No .raw file found"
														exit 1
													fi
													# Check that it's a valid disk image (has non-zero size)
													if [ ! -s "$raw_file" ]; then
														echo "Raw file is empty"
														exit 1
													fi
													# Check file size is reasonable (at least 100MB)
													size=$(stat -c%s "$raw_file")
													if [ "$size" -lt 104857600 ]; then
														echo "Raw file too small: $size bytes"
														exit 1
													fi
													echo "Raw disk verification passed: $raw_file"
												`,
											},
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

		It("builds a valid raw disk image", func() {
			tc.WaitForBuildCompletion(artifactName, artifactLabelSelector)
			tc.WaitForExportCompletion(artifactLabelSelector)
			tc.Cleanup(artifactName, artifactLabelSelector)
		})
	})

	Describe("Netboot", func() {
		var artifactName string
		var artifactLabelSelector labels.Selector

		BeforeEach(func() {
			artifact := &osbuilder.OSArtifact{
				TypeMeta: metav1.TypeMeta{
					Kind:       "OSArtifact",
					APIVersion: osbuilder.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "netboot-",
				},
				Spec: osbuilder.OSArtifactSpec{
					ImageName:  "quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.6.0",
					ISO:        true,
					Netboot:    true,
					NetbootURL: "http://example.com",
					Exporters: []batchv1.JobSpec{
						{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									RestartPolicy: corev1.RestartPolicyNever,
									Containers: []corev1.Container{
										{
											Name:    "verify",
											Image:   "debian:latest",
											Command: []string{"bash"},
											Args: []string{
												"-xec",
												`
													set -e
													# Check for kernel file (pattern: *-kernel)
													kernel_file=$(ls /artifacts/*-kernel 2>/dev/null | head -n1)
													if [ -z "$kernel_file" ]; then
														echo "No kernel file found (pattern: *-kernel)"
														ls -la /artifacts/ || true
														exit 1
													fi
													# Check for initrd file (pattern: *-initrd)
													initrd_file=$(ls /artifacts/*-initrd 2>/dev/null | head -n1)
													if [ -z "$initrd_file" ]; then
														echo "No initrd file found (pattern: *-initrd)"
														ls -la /artifacts/ || true
														exit 1
													fi
													# Check for squashfs file (pattern: *.squashfs)
													squashfs_file=$(ls /artifacts/*.squashfs 2>/dev/null | head -n1)
													if [ -z "$squashfs_file" ]; then
														echo "No squashfs file found (pattern: *.squashfs)"
														ls -la /artifacts/ || true
														exit 1
													fi
													# Verify files are non-empty
													for file in "$kernel_file" "$initrd_file" "$squashfs_file"; do
														if [ ! -s "$file" ]; then
															echo "File is empty: $file"
															exit 1
														fi
													done
													echo "Netboot artifacts verification passed"
													echo "Kernel: $kernel_file"
													echo "Initrd: $initrd_file"
													echo "Squashfs: $squashfs_file"
												`,
											},
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

		It("builds valid netboot artifacts", func() {
			tc.WaitForBuildCompletion(artifactName, artifactLabelSelector)
			tc.WaitForExportCompletion(artifactLabelSelector)
			tc.Cleanup(artifactName, artifactLabelSelector)
		})
	})

	Describe("AzureImage (VHD)", func() {
		var artifactName string
		var artifactLabelSelector labels.Selector

		BeforeEach(func() {
			artifact := &osbuilder.OSArtifact{
				TypeMeta: metav1.TypeMeta{
					Kind:       "OSArtifact",
					APIVersion: osbuilder.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "azure-",
				},
				Spec: osbuilder.OSArtifactSpec{
					ImageName:  "quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.6.0",
					AzureImage: true,
					Exporters: []batchv1.JobSpec{
						{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									RestartPolicy: corev1.RestartPolicyNever,
									Containers: []corev1.Container{
										{
											Name:    "verify",
											Image:   "debian:latest",
											Command: []string{"bash"},
											Args: []string{
												"-xec",
												`
													set -e
													# Check that VHD file exists
													vhd_file=$(ls /artifacts/*.vhd 2>/dev/null | head -n1)
													if [ -z "$vhd_file" ]; then
														echo "No .vhd file found"
														exit 1
													fi
													# Check that it's non-empty
													if [ ! -s "$vhd_file" ]; then
														echo "VHD file is empty"
														exit 1
													fi
													# Check file size is reasonable (at least 100MB)
													size=$(stat -c%s "$vhd_file")
													if [ "$size" -lt 104857600 ]; then
														echo "VHD file too small: $size bytes"
														exit 1
													fi
													# Check VHD footer (last 512 bytes should contain VHD signature)
													# VHD footer starts at offset -512 and contains "conectix" string
													tail -c 512 "$vhd_file" | grep -q "conectix" || {
														echo "VHD file does not have valid VHD footer"
														exit 1
													}
													echo "VHD verification passed: $vhd_file"
												`,
											},
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

		It("builds a valid Azure VHD image", func() {
			tc.WaitForBuildCompletion(artifactName, artifactLabelSelector)
			tc.WaitForExportCompletion(artifactLabelSelector)
			tc.Cleanup(artifactName, artifactLabelSelector)
		})
	})

	Describe("GCEImage", func() {
		var artifactName string
		var artifactLabelSelector labels.Selector

		BeforeEach(func() {
			artifact := &osbuilder.OSArtifact{
				TypeMeta: metav1.TypeMeta{
					Kind:       "OSArtifact",
					APIVersion: osbuilder.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "gce-",
				},
				Spec: osbuilder.OSArtifactSpec{
					ImageName: "quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.6.0",
					GCEImage:  true,
					Exporters: []batchv1.JobSpec{
						{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									RestartPolicy: corev1.RestartPolicyNever,
									Containers: []corev1.Container{
										{
											Name:    "verify",
											Image:   "debian:latest",
											Command: []string{"bash"},
											Args: []string{
												"-xec",
												`
													set -e
													# Check that GCE tar.gz file exists
													gce_file=$(ls /artifacts/*.gce.tar.gz 2>/dev/null | head -n1)
													if [ -z "$gce_file" ]; then
														echo "No .gce.tar.gz file found"
														exit 1
													fi
													# Check that it's non-empty
													if [ ! -s "$gce_file" ]; then
														echo "GCE tar.gz file is empty"
														exit 1
													fi
													# Extract and verify it contains disk.raw
													temp_dir=$(mktemp -d)
													trap "rm -rf $temp_dir" EXIT
													tar -xzf "$gce_file" -C "$temp_dir"
													if [ ! -f "$temp_dir/disk.raw" ]; then
														echo "GCE archive does not contain disk.raw"
														exit 1
													fi
													# Verify disk.raw is non-empty and reasonable size
													if [ ! -s "$temp_dir/disk.raw" ]; then
														echo "disk.raw in archive is empty"
														exit 1
													fi
													size=$(stat -c%s "$temp_dir/disk.raw")
													if [ "$size" -lt 104857600 ]; then
														echo "disk.raw too small: $size bytes"
														exit 1
													fi
													echo "GCE verification passed: $gce_file"
												`,
											},
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

		It("builds a valid GCE image", func() {
			tc.WaitForBuildCompletion(artifactName, artifactLabelSelector)
			tc.WaitForExportCompletion(artifactLabelSelector)
			tc.Cleanup(artifactName, artifactLabelSelector)
		})
	})
})
