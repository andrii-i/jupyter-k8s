//go:build e2e
// +build e2e

/*
Copyright 2025.

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

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-ai-contrib/jupyter-k8s/test/utils"
)

var _ = Describe("WorkspaceTemplate Namespace Resolution", Ordered, func() {
	const (
		sharedNamespace   = "jupyter-k8s-shared"
		teamANamespace    = "team-a"
		teamBNamespace    = "team-b"
		platformNamespace = "platform-templates"

		workspaceCreationTimeout = 30 * time.Second
		workspaceDeletionTimeout = 120 * time.Second
		finalizerTimeout         = 60 * time.Second
	)

	getWorkspaceImage := func(workspaceName, workspaceNamespace string) (string, error) {
		cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
			"-n", workspaceNamespace,
			"-o", "jsonpath={.spec.image}")
		return utils.Run(cmd)
	}

	getWorkspaceTemplateNamespaceLabel := func(workspaceName, workspaceNamespace string) (string, error) {
		cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
			"-n", workspaceNamespace,
			"-o", "jsonpath={.metadata.labels.workspace\\.jupyter\\.org/template-namespace}")
		return utils.Run(cmd)
	}

	waitForWorkspaceCreated := func(workspaceName, workspaceNamespace string) {
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "workspace", workspaceName, "-n", workspaceNamespace)
			_, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred(), "workspace should exist")
		}).WithTimeout(workspaceCreationTimeout).WithPolling(1 * time.Second).Should(Succeed())
	}

	// Helper function to delete workspace
	deleteWorkspace := func(workspaceName, workspaceNamespace string) {
		By(fmt.Sprintf("deleting workspace %s/%s", workspaceNamespace, workspaceName))
		cmd := exec.Command("kubectl", "delete", "workspace", workspaceName,
			"-n", workspaceNamespace, "--ignore-not-found", "--wait", "--timeout=60s")
		_, _ = utils.Run(cmd)

		// Wait for deletion to complete
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "workspace", workspaceName, "-n", workspaceNamespace)
			_, err := utils.Run(cmd)
			g.Expect(err).To(HaveOccurred(), "workspace should be deleted")
		}).WithTimeout(workspaceDeletionTimeout).WithPolling(2 * time.Second).Should(Succeed())
	}

	// Helper function to create namespace
	createNamespace := func(namespace string) {
		By(fmt.Sprintf("creating namespace %s", namespace))
		cmd := exec.Command("kubectl", "create", "namespace", namespace)
		_, err := utils.Run(cmd)
		if err != nil && !strings.Contains(err.Error(), "AlreadyExists") {
			Expect(err).NotTo(HaveOccurred())
		}
	}

	// Helper function to delete namespace
	deleteNamespace := func(namespace string) {
		By(fmt.Sprintf("deleting namespace %s", namespace))
		cmd := exec.Command("kubectl", "delete", "namespace", namespace, "--ignore-not-found", "--timeout=120s")
		_, _ = utils.Run(cmd)
	}

	BeforeAll(func() {
		var err error

		// Create test namespaces
		createNamespace(teamANamespace)
		createNamespace(teamBNamespace)
		createNamespace(platformNamespace)

		// Ensure shared namespace exists (should already exist from other tests)
		createNamespace(sharedNamespace)

		// Apply templates to appropriate namespaces
		By("applying template for workspace namespace default test")
		cmd := exec.Command("kubectl", "apply", "-f",
			"test/e2e/static/template-namespace/template-team-a-basic.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("applying template for fallback test")
		cmd = exec.Command("kubectl", "apply", "-f",
			"test/e2e/static/template-namespace/template-shared-basic.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("applying templates for priority test")
		cmd = exec.Command("kubectl", "apply", "-f",
			"test/e2e/static/template-namespace/template-team-a-priority.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		cmd = exec.Command("kubectl", "apply", "-f",
			"test/e2e/static/template-namespace/template-shared-priority.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("applying template for cross-namespace test")
		cmd = exec.Command("kubectl", "apply", "-f",
			"test/e2e/static/template-namespace/template-platform-cross-ns.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		// Wait a bit for templates to be ready
		time.Sleep(2 * time.Second)
	})

	AfterAll(func() {
		By("cleaning up test resources")

		// Delete workspaces (if any remain) with wait to ensure clean deletion
		_ = exec.Command("kubectl", "delete", "workspace", "--all", "-n", teamANamespace, "--wait", "--timeout=60s", "--ignore-not-found").Run()
		_ = exec.Command("kubectl", "delete", "workspace", "--all", "-n", teamBNamespace, "--wait", "--timeout=60s", "--ignore-not-found").Run()

		// Delete templates with wait
		_ = exec.Command("kubectl", "delete", "workspacetemplate", "basic-template", "-n", teamANamespace, "--wait", "--timeout=30s", "--ignore-not-found").Run()
		_ = exec.Command("kubectl", "delete", "workspacetemplate", "fallback-template", "-n", sharedNamespace, "--wait", "--timeout=30s", "--ignore-not-found").Run()
		_ = exec.Command("kubectl", "delete", "workspacetemplate", "priority-test-template", "-n", teamANamespace, "--wait", "--timeout=30s", "--ignore-not-found").Run()
		_ = exec.Command("kubectl", "delete", "workspacetemplate", "priority-test-template", "-n", sharedNamespace, "--wait", "--timeout=30s", "--ignore-not-found").Run()
		_ = exec.Command("kubectl", "delete", "workspacetemplate", "platform-shared-template", "-n", platformNamespace, "--wait", "--timeout=30s", "--ignore-not-found").Run()

		// Delete test namespaces
		deleteNamespace(teamANamespace)
		deleteNamespace(teamBNamespace)
		deleteNamespace(platformNamespace)
	})

	Context("Empty templateRef.namespace - Workspace Namespace Default", func() {
		const (
			workspaceName = "test-empty-ns-workspace-team-a"
		)

		It("should resolve template from workspace namespace when templateRef.namespace is empty", func() {
			var err error
			var output string

			By("creating workspace with empty templateRef.namespace")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-namespace/workspace-empty-ns-team-a.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			waitForWorkspaceCreated(workspaceName, teamANamespace)

			By("verifying template namespace label matches workspace namespace")
			output, err = getWorkspaceTemplateNamespaceLabel(workspaceName, teamANamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(teamANamespace))

			By("verifying template defaults were applied")
			output, err = getWorkspaceImage(workspaceName, teamANamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("jupyter/base-notebook:team-a"))

			// Cleanup
			deleteWorkspace(workspaceName, teamANamespace)
		})
	})

	Context("Empty templateRef.namespace - Default Namespace Fallback", func() {
		const (
			workspaceName = "test-fallback-workspace"
		)

		It("should fallback to default namespace when template not in workspace namespace", func() {
			var err error
			var output string

			By("creating workspace - template only in shared namespace, not in team-a")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-namespace/workspace-fallback-to-shared.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			waitForWorkspaceCreated(workspaceName, teamANamespace)

			By("verifying fallback to shared namespace template")
			output, err = getWorkspaceImage(workspaceName, teamANamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("jupyter/base-notebook:shared"))

			// Cleanup
			deleteWorkspace(workspaceName, teamANamespace)
		})
	})

	Context("Namespace Priority - Workspace Over Default", func() {
		const (
			workspaceName = "test-priority-workspace"
		)

		It("should prioritize workspace namespace over default namespace", func() {
			var err error
			var output string

			By("verifying template exists in both namespaces with same name but different images")
			cmd := exec.Command("kubectl", "get", "workspacetemplate", "priority-test-template",
				"-n", teamANamespace, "-o", "jsonpath={.spec.defaultImage}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("jupyter/base-notebook:team-a-priority"))

			cmd = exec.Command("kubectl", "get", "workspacetemplate", "priority-test-template",
				"-n", sharedNamespace, "-o", "jsonpath={.spec.defaultImage}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("jupyter/base-notebook:shared-priority"))

			By("creating workspace - template exists in both team-a and shared namespaces")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-namespace/workspace-priority-test.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			waitForWorkspaceCreated(workspaceName, teamANamespace)

			By("verifying workspace namespace template takes priority over default")
			output, err = getWorkspaceImage(workspaceName, teamANamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("jupyter/base-notebook:team-a-priority"))

			// Cleanup
			deleteWorkspace(workspaceName, teamANamespace)
		})
	})

	Context("Explicit Cross-Namespace Reference", func() {
		const (
			workspaceName = "test-cross-ns-workspace"
		)

		It("should resolve template from explicitly specified namespace", func() {
			var err error
			var output string

			By("creating workspace with explicit cross-namespace templateRef")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-namespace/workspace-explicit-cross-ns.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			waitForWorkspaceCreated(workspaceName, teamBNamespace)

			By("verifying template from explicitly specified namespace")
			output, err = getWorkspaceImage(workspaceName, teamBNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("jupyter/base-notebook:platform"))

			// Cleanup
			deleteWorkspace(workspaceName, teamBNamespace)
		})
	})

	Context("Template Not Found - All Tiers Exhausted", func() {
		It("should reject workspace when template not found in any namespace", func() {
			By("attempting to create workspace with nonexistent template")
			workspaceYAML := `
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: test-not-found-workspace
  namespace: team-a
spec:
  displayName: "Test Template Not Found"
  ownershipType: Public
  desiredStatus: Running
  templateRef:
    name: nonexistent-template
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(workspaceYAML)
			output, err := utils.Run(cmd)

			By("verifying workspace creation was rejected")
			// Should be rejected by validation webhook
			Expect(err).To(HaveOccurred(), "workspace with nonexistent template should be rejected")
			Expect(output).To(ContainSubstring("failed to get template"), "error should mention template not found")

			// Cleanup (in case it somehow got created)
			_ = exec.Command("kubectl", "delete", "workspace", "test-not-found-workspace",
				"-n", teamANamespace, "--ignore-not-found").Run()
		})
	})

	Context("Explicit Wrong Namespace", func() {
		It("should reject workspace when template not found in explicit namespace", func() {
			By("attempting to create workspace with explicit wrong namespace")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-namespace/workspace-explicit-wrong-ns.yaml")
			output, err := utils.Run(cmd)

			By("verifying workspace creation was rejected by validation webhook")
			Expect(err).To(HaveOccurred(), "workspace with wrong explicit namespace should be rejected")
			Expect(output).To(ContainSubstring("failed to get template"), "error should mention template not found")
			Expect(output).To(ContainSubstring("team-b"), "error should mention the explicit namespace tried")

			// Cleanup (in case it somehow got created)
			_ = exec.Command("kubectl", "delete", "workspace", "test-explicit-wrong-ns",
				"-n", teamANamespace, "--ignore-not-found").Run()
		})
	})

	Context("Finalizer Cross-Namespace Behavior", func() {
		const (
			templateName      = "cross-ns-finalizer-template"
			workspaceName     = "cross-ns-finalizer-workspace"
			finalizerName     = "workspace.jupyter.org/template-protection"
		)

		It("should protect cross-namespace templates from deletion", func() {
			var err error
			var output string

			By("creating template in shared namespace")
			templateYAML := `
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: cross-ns-finalizer-template
  namespace: jupyter-k8s-shared
spec:
  displayName: "Cross-Namespace Finalizer Test"
  defaultImage: "jupyter/base-notebook:finalizer-test"
  defaultStorageSize: "5Gi"
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(templateYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace in team-a referencing template in shared namespace")
			workspaceYAML := `
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: cross-ns-finalizer-workspace
  namespace: team-a
spec:
  displayName: "Cross-NS Finalizer Workspace"
  ownershipType: Public
  desiredStatus: Running
  templateRef:
    name: cross-ns-finalizer-template
    namespace: jupyter-k8s-shared
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(workspaceYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			waitForWorkspaceCreated(workspaceName, teamANamespace)

			By("waiting for finalizer to be added to template in shared namespace")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspacetemplate", templateName,
					"-n", sharedNamespace, "-o", "jsonpath={.metadata.finalizers}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring(finalizerName), "finalizer should be added")
			}).WithTimeout(30 * time.Second).WithPolling(1 * time.Second).Should(Succeed())

			By("attempting to delete template while workspace exists")
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", templateName,
				"-n", sharedNamespace, "--wait=false")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying template has deletionTimestamp but still exists")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspacetemplate", templateName,
					"-n", sharedNamespace, "-o", "jsonpath={.metadata.deletionTimestamp}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "deletionTimestamp should be set")
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			By("verifying finalizer is still present, blocking deletion")
			cmd = exec.Command("kubectl", "get", "workspacetemplate", templateName,
				"-n", sharedNamespace, "-o", "jsonpath={.metadata.finalizers}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring(finalizerName), "finalizer should still be present")

			By("deleting workspace")
			deleteWorkspace(workspaceName, teamANamespace)

			By("verifying template can now be deleted after workspace removal")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspacetemplate", templateName, "-n", sharedNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "template should be deleted")
			}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})
	})
})
