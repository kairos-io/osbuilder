package controllers

import (
	"context"

	osbuilder "github.com/kairos-io/osbuilder/api/v1alpha2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("OSArtifactReconciler", func() {
	var r *OSArtifactReconciler
	var artifact *osbuilder.OSArtifact

	BeforeEach(func() {
		r = &OSArtifactReconciler{}
		artifact = &osbuilder.OSArtifact{}

		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme: runtime.NewScheme(),
		})
		Expect(err).ToNot(HaveOccurred())

		err = (r).SetupWithManager(mgr)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("CreateConfigMap", func() {
		It("test", func() {
			ctx := context.Background()
			err := r.CreateConfigMap(ctx, artifact)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
