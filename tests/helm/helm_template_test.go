/**
# Copyright 2024 NVIDIA CORPORATION
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package helm_test

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/gruntwork-io/terratest/modules/helm"
	"github.com/gruntwork-io/terratest/modules/logger"

	"github.com/gruntwork-io/terratest/modules/k8s"
)

func TestDevicePluginDaemonsetTemplateRenderedDeployment(t *testing.T) {
	// Path to the helm chart we will test
	helmChartPath, err := filepath.Abs("../../deployments/helm/nvidia-device-plugin")
	releaseName := "nvidia-device-plugin"
	require.NoError(t, err)

	// Since we aren't deploying any resources, there is no need to setup kubectl authentication or helm home.

	testCases := []struct {
		description string
		options     map[string]string
		// TODO: We should find a better way to define the expected
		expectedContainer v1.Container
	}{
		{
			description: "no options",
			expectedContainer: v1.Container{
				SecurityContext: &v1.SecurityContext{
					AllowPrivilegeEscalation: ptr(false),
					Capabilities: &v1.Capabilities{
						Drop: []v1.Capability{"ALL"},
					},
				},
			},
		},
		{
			description: "set compatWithCPUManager",
			options: map[string]string{
				"compatWithCPUManager": "true",
			},
			expectedContainer: v1.Container{
				SecurityContext: &v1.SecurityContext{
					Privileged: ptr(true),
				},
			},
		},
		{
			description: "set mig-strategy to single",
			options: map[string]string{
				"migStrategy": "single",
			},
			expectedContainer: v1.Container{
				SecurityContext: &v1.SecurityContext{
					Capabilities: &v1.Capabilities{
						Add: []v1.Capability{"SYS_ADMIN"},
					},
				},
			},
		},
		{
			description: "set device-list-strategy to volume-mounts",
			options: map[string]string{
				"deviceListStrategy": "volume-mounts",
			},
			expectedContainer: v1.Container{
				SecurityContext: &v1.SecurityContext{
					Capabilities: &v1.Capabilities{
						Add: []v1.Capability{"SYS_ADMIN"},
					},
				},
			},
		},
	}

	for i, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Set up the namespace; confirm that the template renders the expected value for the namespace.
			namespaceName := fmt.Sprintf("k8s-device-plugin-test-%d", i)

			options := &helm.Options{
				SetValues:      tc.options,
				KubectlOptions: k8s.NewKubectlOptions("", "", namespaceName),
				Logger:         logger.Discard,
			}

			// Run RenderTemplate to render the template and capture the output. Note that we use the version without `E`, since
			// we want to assert that the template renders without any errors.
			// Additionally, although we know there is only one yaml file in the template, we deliberately path a templateFiles
			// arg to demonstrate how to select individual templates to render.
			output := helm.RenderTemplate(t, options, helmChartPath, releaseName, []string{"templates/daemonset-device-plugin.yml"})

			// Now we use kubernetes/client-go library to render the template output into the Deployment struct. This will
			// ensure the Deployment resource is rendered correctly.
			var deployment appsv1.Deployment
			helm.UnmarshalK8SYaml(t, output, &deployment)

			require.Equal(t, namespaceName, deployment.Namespace)
			require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

			devicePluginContainer := deployment.Spec.Template.Spec.Containers[0]
			require.EqualValues(t, tc.expectedContainer.SecurityContext, devicePluginContainer.SecurityContext)
		})
	}
}

func TestComponentResourcesTemplateRendered(t *testing.T) {
	helmChartPath, err := filepath.Abs("../../deployments/helm/nvidia-device-plugin")
	require.NoError(t, err)

	templateFiles := []string{
		"templates/daemonset-device-plugin.yml",
		"templates/daemonset-gfd.yml",
		"templates/daemonset-mps-control-daemon.yml",
	}

	testCases := []struct {
		description string
		options     map[string]string
		// An empty memory limit means the container has no resources set.
		expectedMemoryLimitByContainer map[string]string
	}{
		{
			description: "top-level resources apply to main containers only",
			options: map[string]string{
				"resources.limits.memory": "1Gi",
			},
			expectedMemoryLimitByContainer: map[string]string{
				"nvidia-device-plugin-init":     "",
				"nvidia-device-plugin-sidecar":  "",
				"nvidia-device-plugin-ctr":      "1Gi",
				"gpu-feature-discovery-init":    "",
				"gpu-feature-discovery-sidecar": "",
				"gpu-feature-discovery-ctr":     "1Gi",
				"mps-control-daemon-mounts":     "1Gi",
				"mps-control-daemon-init":       "",
				"mps-control-daemon-sidecar":    "",
				"mps-control-daemon-ctr":        "1Gi",
			},
		},
		{
			description: "values reused from a release without configManager",
			options: map[string]string{
				"configManager":           "null",
				"resources.limits.memory": "1Gi",
			},
			expectedMemoryLimitByContainer: map[string]string{
				"nvidia-device-plugin-init":     "",
				"nvidia-device-plugin-sidecar":  "",
				"nvidia-device-plugin-ctr":      "1Gi",
				"gpu-feature-discovery-init":    "",
				"gpu-feature-discovery-sidecar": "",
				"gpu-feature-discovery-ctr":     "1Gi",
				"mps-control-daemon-mounts":     "1Gi",
				"mps-control-daemon-init":       "",
				"mps-control-daemon-sidecar":    "",
				"mps-control-daemon-ctr":        "1Gi",
			},
		},
		{
			description: "component resources override top-level resources",
			options: map[string]string{
				"resources.limits.memory":               "1Gi",
				"devicePlugin.resources.limits.memory":  "3Gi",
				"gfd.resources.limits.memory":           "2Gi",
				"mps.resources.limits.memory":           "4Gi",
				"configManager.resources.limits.memory": "64Mi",
			},
			expectedMemoryLimitByContainer: map[string]string{
				"nvidia-device-plugin-init":     "64Mi",
				"nvidia-device-plugin-sidecar":  "64Mi",
				"nvidia-device-plugin-ctr":      "3Gi",
				"gpu-feature-discovery-init":    "64Mi",
				"gpu-feature-discovery-sidecar": "64Mi",
				"gpu-feature-discovery-ctr":     "2Gi",
				"mps-control-daemon-mounts":     "4Gi",
				"mps-control-daemon-init":       "64Mi",
				"mps-control-daemon-sidecar":    "64Mi",
				"mps-control-daemon-ctr":        "4Gi",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			setValues := map[string]string{
				"config.name": "external-config",
				"gfd.enabled": "true",
			}
			maps.Copy(setValues, tc.options)
			options := &helm.Options{
				SetValues:      setValues,
				KubectlOptions: k8s.NewKubectlOptions("", "", "k8s-device-plugin-test"),
				Logger:         logger.Discard,
			}

			memoryLimitByContainer := make(map[string]string)
			for _, templateFile := range templateFiles {
				output := helm.RenderTemplate(t, options, helmChartPath, "nvidia-device-plugin", []string{templateFile})

				var daemonset appsv1.DaemonSet
				helm.UnmarshalK8SYaml(t, output, &daemonset)

				podSpec := daemonset.Spec.Template.Spec
				for _, container := range append(podSpec.InitContainers, podSpec.Containers...) {
					memoryLimit := ""
					if limit, ok := container.Resources.Limits[v1.ResourceMemory]; ok {
						memoryLimit = limit.String()
					}
					memoryLimitByContainer[container.Name] = memoryLimit
				}
			}

			require.Equal(t, tc.expectedMemoryLimitByContainer, memoryLimitByContainer)
		})
	}
}

func TestDevicePluginDaemonsetNvidiaDriverCapabilities(t *testing.T) {
	helmChartPath, err := filepath.Abs("../../deployments/helm/nvidia-device-plugin")
	require.NoError(t, err)

	testCases := []struct {
		description                   string
		nvidiaDriverCapabilitiesJSON  string
		expectedDriverCapabilitiesEnv *v1.EnvVar
		expectSchemaRejection         bool
	}{
		{
			description:                   "default",
			expectedDriverCapabilitiesEnv: &v1.EnvVar{Name: "NVIDIA_DRIVER_CAPABILITIES", Value: "compute,utility"},
		},
		{
			description:                   "string",
			nvidiaDriverCapabilitiesJSON:  `"all"`,
			expectedDriverCapabilitiesEnv: &v1.EnvVar{Name: "NVIDIA_DRIVER_CAPABILITIES", Value: "all"},
		},
		{
			description:                  "null omits the variable",
			nvidiaDriverCapabilitiesJSON: "null",
		},
		{
			description:                  "empty string omits the variable",
			nvidiaDriverCapabilitiesJSON: `""`,
		},
		{
			description:                  "boolean is rejected",
			nvidiaDriverCapabilitiesJSON: "true",
			expectSchemaRejection:        true,
		},
		{
			description:                  "number is rejected",
			nvidiaDriverCapabilitiesJSON: "1",
			expectSchemaRejection:        true,
		},
		{
			description:                  "list is rejected",
			nvidiaDriverCapabilitiesJSON: `["compute","utility"]`,
			expectSchemaRejection:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			options := &helm.Options{
				KubectlOptions: k8s.NewKubectlOptions("", "", "k8s-device-plugin-test"),
				Logger:         logger.Discard,
			}
			if tc.nvidiaDriverCapabilitiesJSON != "" {
				options.SetJsonValues = map[string]string{"nvidiaDriverCapabilities": tc.nvidiaDriverCapabilitiesJSON}
			}

			// values.schema.json is enforced even when only the daemonset is selected for output.
			output, err := helm.RenderTemplateE(t, options, helmChartPath, "nvidia-device-plugin", []string{"templates/daemonset-device-plugin.yml"})
			if tc.expectSchemaRejection {
				requireSchemaRejection(t, err, "nvidiaDriverCapabilities")
				return
			}
			require.NoError(t, err)

			var daemonset appsv1.DaemonSet
			helm.UnmarshalK8SYaml(t, output, &daemonset)
			require.Len(t, daemonset.Spec.Template.Spec.Containers, 1)

			var driverCapabilitiesEnv *v1.EnvVar
			for _, env := range daemonset.Spec.Template.Spec.Containers[0].Env {
				if env.Name == "NVIDIA_DRIVER_CAPABILITIES" {
					driverCapabilitiesEnv = &env
				}
			}
			require.Equal(t, tc.expectedDriverCapabilitiesEnv, driverCapabilitiesEnv)
		})
	}
}

func TestDevicePluginDaemonsetImageTag(t *testing.T) {
	helmChartPath, err := filepath.Abs("../../deployments/helm/nvidia-device-plugin")
	require.NoError(t, err)

	testCases := []struct {
		description   string
		imageTag      string
		expectedImage string
	}{
		{
			description:   "string tag",
			imageTag:      "v0.20.1",
			expectedImage: "nvcr.io/nvidia/k8s-device-plugin:v0.20.1",
		},
		{
			// --set parses an all-digit tag as a number.
			description:   "numeric tag",
			imageTag:      "123",
			expectedImage: "nvcr.io/nvidia/k8s-device-plugin:123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			options := &helm.Options{
				SetValues:      map[string]string{"image.tag": tc.imageTag},
				KubectlOptions: k8s.NewKubectlOptions("", "", "k8s-device-plugin-test"),
				Logger:         logger.Discard,
			}

			output := helm.RenderTemplate(t, options, helmChartPath, "nvidia-device-plugin", []string{"templates/daemonset-device-plugin.yml"})

			var daemonset appsv1.DaemonSet
			helm.UnmarshalK8SYaml(t, output, &daemonset)
			require.Len(t, daemonset.Spec.Template.Spec.Containers, 1)
			require.Equal(t, tc.expectedImage, daemonset.Spec.Template.Spec.Containers[0].Image)
		})
	}
}

func TestComponentHostNetworkTemplateRendered(t *testing.T) {
	helmChartPath, err := filepath.Abs("../../deployments/helm/nvidia-device-plugin")
	require.NoError(t, err)

	templateFileByComponent := map[string]string{
		"devicePlugin": "templates/daemonset-device-plugin.yml",
		"gfd":          "templates/daemonset-gfd.yml",
		"mps":          "templates/daemonset-mps-control-daemon.yml",
	}

	testCases := []struct {
		description           string
		setValue              string
		setStringValue        string
		expectedHostNetwork   bool
		expectSchemaRejection bool
	}{
		{
			description: "default",
		},
		{
			description:         "true",
			setValue:            "true",
			expectedHostNetwork: true,
		},
		{
			description:           "string is rejected",
			setStringValue:        "false",
			expectSchemaRejection: true,
		},
	}

	for component, templateFile := range templateFileByComponent {
		for _, tc := range testCases {
			t.Run(component+"/"+tc.description, func(t *testing.T) {
				hostNetworkKey := component + ".enableHostNetwork"
				options := &helm.Options{
					SetValues: map[string]string{
						"config.name": "external-config",
						"gfd.enabled": "true",
					},
					SetStrValues:   map[string]string{},
					KubectlOptions: k8s.NewKubectlOptions("", "", "k8s-device-plugin-test"),
					Logger:         logger.Discard,
				}
				if tc.setValue != "" {
					options.SetValues[hostNetworkKey] = tc.setValue
				}
				if tc.setStringValue != "" {
					options.SetStrValues[hostNetworkKey] = tc.setStringValue
				}

				output, err := helm.RenderTemplateE(t, options, helmChartPath, "nvidia-device-plugin", []string{templateFile})
				if tc.expectSchemaRejection {
					requireSchemaRejection(t, err, hostNetworkKey)
					return
				}
				require.NoError(t, err)

				var daemonset appsv1.DaemonSet
				helm.UnmarshalK8SYaml(t, output, &daemonset)
				require.Equal(t, tc.expectedHostNetwork, daemonset.Spec.Template.Spec.HostNetwork)
			})
		}
	}
}

func TestGFDDaemonsetEnvTemplateRendered(t *testing.T) {
	helmChartPath, err := filepath.Abs("../../deployments/helm/nvidia-device-plugin")
	require.NoError(t, err)

	testCases := []struct {
		description           string
		options               map[string]string
		expectedEnvByName     map[string]string
		expectSchemaRejection bool
	}{
		{
			description:       "default",
			expectedEnvByName: map[string]string{},
		},
		{
			description: "gfd values",
			options: map[string]string{
				"gfd.noTimestamp":   "true",
				"gfd.sleepInterval": "30s",
			},
			expectedEnvByName: map[string]string{
				"GFD_NO_TIMESTAMP":   "true",
				"GFD_SLEEP_INTERVAL": "30s",
			},
		},
		{
			// GFD parses GFD_SLEEP_INTERVAL with time.ParseDuration, so a number is never a valid interval.
			description: "numeric sleepInterval is rejected",
			options: map[string]string{
				"gfd.sleepInterval": "60",
			},
			expectSchemaRejection: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			setValues := map[string]string{
				"gfd.enabled": "true",
			}
			maps.Copy(setValues, tc.options)
			options := &helm.Options{
				SetValues:      setValues,
				KubectlOptions: k8s.NewKubectlOptions("", "", "k8s-device-plugin-test"),
				Logger:         logger.Discard,
			}

			output, err := helm.RenderTemplateE(t, options, helmChartPath, "nvidia-device-plugin", []string{"templates/daemonset-gfd.yml"})
			if tc.expectSchemaRejection {
				requireSchemaRejection(t, err, "gfd.sleepInterval")
				return
			}
			require.NoError(t, err)

			var daemonset appsv1.DaemonSet
			helm.UnmarshalK8SYaml(t, output, &daemonset)
			require.Len(t, daemonset.Spec.Template.Spec.Containers, 1)

			envByName := map[string]string{}
			for _, env := range daemonset.Spec.Template.Spec.Containers[0].Env {
				if env.Name == "GFD_NO_TIMESTAMP" || env.Name == "GFD_SLEEP_INTERVAL" {
					envByName[env.Name] = env.Value
				}
			}
			require.Equal(t, tc.expectedEnvByName, envByName)
		})
	}
}

func TestDevicePluginDaemonsetAffinityTemplateRendered(t *testing.T) {
	helmChartPath, err := filepath.Abs("../../deployments/helm/nvidia-device-plugin")
	require.NoError(t, err)

	testCases := []struct {
		description      string
		options          map[string]string
		expectedAffinity bool
	}{
		{
			description:      "default",
			expectedAffinity: true,
		},
		{
			description:      "null clears the default",
			options:          map[string]string{"affinity": "null"},
			expectedAffinity: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			options := &helm.Options{
				SetValues:      tc.options,
				KubectlOptions: k8s.NewKubectlOptions("", "", "k8s-device-plugin-test"),
				Logger:         logger.Discard,
			}

			output := helm.RenderTemplate(t, options, helmChartPath, "nvidia-device-plugin", []string{"templates/daemonset-device-plugin.yml"})

			var daemonset appsv1.DaemonSet
			helm.UnmarshalK8SYaml(t, output, &daemonset)
			require.Equal(t, tc.expectedAffinity, daemonset.Spec.Template.Spec.Affinity != nil)
		})
	}
}

// Helm 3.18 switched JSON schema validators, so the error names a value as
// "/gfd/sleepInterval" from then on and as "gfd.sleepInterval" before it.
func requireSchemaRejection(t *testing.T, err error, valuePath string) {
	t.Helper()
	require.ErrorContains(t, err, "values don't meet the specifications of the schema")
	jsonPointer := "/" + strings.ReplaceAll(valuePath, ".", "/")
	require.Truef(t, strings.Contains(err.Error(), jsonPointer) || strings.Contains(err.Error(), valuePath+":"),
		"schema error does not name %q: %v", valuePath, err)
}

func ptr[T any](x T) *T {
	return &x
}
