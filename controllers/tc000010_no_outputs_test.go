package controllers

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_0000010_no_outputs_test(t *testing.T) {
	const (
		sourceName    = "test-tf-controller-no-output"
		terraformName = "helloworld-no-outputs"
	)

	g := NewWithT(t)

	// When create a hello world TF object with auto approve
	// should run the TF hello world program from the BLOB and get the correct output as a secret
	ctx := context.Background()

	by("creating a new Git repository object")
	testRepo := sourcev1.GitRepository{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "source.toolkit.fluxcd.io/v1beta1",
			Kind:       "GitRepository",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "master",
			},
			Interval:          metav1.Duration{Duration: time.Second * 30},
			GitImplementation: "go-git",
		},
	}
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())

	by("setting the git repo status object, the URL, and the correct checksum")
	updatedTime := time.Now()
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},
		URL: server.URL() + "/file.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	by("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

	by("creating a new TF and attaching to the repo")
	helloWorldTF := infrav1.Terraform{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "infra.contrib.fluxcd.io/v1alpha1",
			Kind:       "Terraform",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())

	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}

	by("checking that the hello world TF got created")
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	by("checking that the hello world TF's status has some conditions")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	by("checking that the planned status of the TF program is Planned, Successfully")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"Type":    createdHelloWorldTF.Status.Conditions[0].Type,
			"Reason":  createdHelloWorldTF.Status.Conditions[0].Reason,
			"Message": createdHelloWorldTF.Status.Conditions[0].Message,
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    "Plan",
		"Reason":  "TerraformPlannedSucceed",
		"Message": "Terraform Plan Generated Successfully",
	}))

	by("checking that there is a secret containing plan-master-b8e362c206")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	tfplanSecret := corev1.Secret{}
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"SavedPlan":   tfplanSecret.Labels["savedPlan"],
			"TFPlanEmpty": string(tfplanSecret.Data["tfplan"]) == "",
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"SavedPlan":   "plan-master-b8e362c206",
		"TFPlanEmpty": false,
	}))

	by("checking that the applied status of the TF program Successfully, and plan-1 is applied")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            "Apply",
		"Reason":          "TerraformAppliedSucceed",
		"LastAppliedPlan": "plan-master-b8e362c206",
	}))
	// TODO check Output condition

	by("checking that we have outputs available in the TF object")
	g.Eventually(func() []string {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		return createdHelloWorldTF.Status.AvailableOutputs
	}, timeout, interval).Should(Equal([]string{"hello_world"}))

	// We're testing out side Kubernetes
	if os.Getenv("DISABLE_TF_K8S_BACKEND") == "1" {
		// So we're expecting that there must be no "tfstate-default-${terraformName}" secret
		by("checking that we're testing the controller locally, then there should be no secret generated by defailt")
		tfStateKey := types.NamespacedName{Namespace: "flux-system", Name: "tfstate-default-" + terraformName}
		tfStateSecret := corev1.Secret{}
		g.Eventually(func() string {
			err := k8sClient.Get(ctx, tfStateKey, &tfStateSecret)
			if err != nil {
				return err.Error()
			}
			return tfStateSecret.Name
		}, timeout, interval).Should(Equal("secrets \"tfstate-default-helloworld-no-outputs\" not found"))
	} else {
		// TODO there's must be the default tfstate secret
	}
}
