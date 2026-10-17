/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package application

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Trait appliesToWorkloads validation", func() {
	BeforeEach(func() {
		handler = &ValidatingHandler{
			Client:  k8sClient,
			Decoder: decoder,
		}
	})

	DescribeTable("rule matching",
		func(rules []string, workload string, want bool) {
			Expect(traitAppliesToWorkload(rules, workload)).To(Equal(want))
		},
		Entry("no rules allows every workload", nil, "deployments.apps", true),
		Entry("wildcard", []string{"*"}, "deployments.apps", true),
		Entry("exact workload", []string{"deployments.apps"}, "deployments.apps", true),
		Entry("other workload", []string{"statefulsets.apps"}, "deployments.apps", false),
		Entry("group wildcard", []string{"*.apps"}, "deployments.apps", true),
		Entry("group wildcard is a whole group", []string{"*.apps"}, "deployments.extensions", false),
		Entry("one matching rule is enough", []string{"statefulsets.apps", "deployments.apps"}, "deployments.apps", true),
	)

	It("rejects a trait whose appliesToWorkloads excludes the component's workload", func() {
		// worker is a Deployment (apps/v1) in the suite's ComponentDefinition.
		stsOnly := &v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "sts-only-scaler", Namespace: oam.SystemDefinitionNamespace},
			Spec:       v1beta1.TraitDefinitionSpec{AppliesToWorkloads: []string{"statefulsets.apps"}},
		}
		Expect(k8sClient.Create(ctx, stsOnly)).To(Succeed())

		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object: runtime.RawExtension{Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"applies-to-repro","namespace":"default"},
"spec":{"components":[{"name":"web","type":"worker","properties":{"image":"nginx:stable"},
"traits":[{"type":"sts-only-scaler","properties":{"replicas":3}}]}]}}
`)},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).To(BeFalse())
		Expect(resp.Result.Message).To(ContainSubstring(
			`trait "sts-only-scaler" cannot apply to workload "deployments.apps" of component "web" (appliable: ["statefulsets.apps"])`))
	})

	It("admits a trait that applies to the component's workload", func() {
		// scaler applies to deployments.apps, so it belongs on worker.
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object: runtime.RawExtension{Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"applies-to-allowed","namespace":"default"},
"spec":{"components":[{"name":"web","type":"worker","properties":{"image":"nginx:stable"},
"traits":[{"type":"scaler","properties":{"replicas":3}}]}]}}
`)},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).To(BeTrue(), resp.Result.Message)
	})
})
