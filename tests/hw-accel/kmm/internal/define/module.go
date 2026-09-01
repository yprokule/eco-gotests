package define

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	moduleV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ModuleLoaderSpec builds the unstructured moduleLoader portion of a Module spec,
// including a single kernel mapping with an in-cluster build.
func ModuleLoaderSpec(kmodName, image, buildArgValue,
	serviceAccountName string) map[string]interface{} {
	return map[string]interface{}{
		"container": map[string]interface{}{
			"modprobe": map[string]interface{}{
				"moduleName": kmodName,
			},
			"kernelMappings": []interface{}{
				map[string]interface{}{
					"regexp":         "^.+$",
					"containerImage": image,
					"build": map[string]interface{}{
						"buildArgs": []interface{}{
							map[string]interface{}{
								"name":  kmmparams.BuildArgName,
								"value": buildArgValue,
							},
						},
						"dockerfileConfigMap": map[string]interface{}{
							"name": kmodName,
						},
					},
				},
			},
		},
		"serviceAccountName": serviceAccountName,
	}
}

// DRAContainer builds the unstructured spec.dra.container for the example DRA driver.
// extraEnv is appended after DRIVER_NAME. HEALTHCHECK_PORT uses
// kmmparams.DRAHealthcheckPort so the hostNetwork plugin does not bind KMM's
// default 51515, which is already used by other DRA drivers (for example Neuron).
func DRAContainer(extraEnv []map[string]interface{}) map[string]interface{} {
	env := []interface{}{
		map[string]interface{}{
			"name":  "DRIVER_NAME",
			"value": kmmparams.DRADriverName,
		},
	}

	for _, e := range extraEnv {
		env = append(env, e)
	}

	env = append(env, map[string]interface{}{
		"name":  "HEALTHCHECK_PORT",
		"value": fmt.Sprintf("%d", kmmparams.DRAHealthcheckPort),
	})

	return map[string]interface{}{
		"image":   kmmparams.DRADriverImage,
		"command": []interface{}{"dra-example-kubeletplugin"},
		"env":     env,
		"livenessProbe": map[string]interface{}{
			"grpc": map[string]interface{}{
				"port":    kmmparams.DRAHealthcheckPort,
				"service": "liveness",
			},
			"initialDelaySeconds": 30,
			"periodSeconds":       10,
			"timeoutSeconds":      5,
			"failureThreshold":    3,
		},
	}
}

// DRASpec builds the unstructured dra portion of a Module spec.
// deviceClassNames may be nil for modules with no deviceClasses.
// extraEnv is appended after the default DRIVER_NAME env var.
func DRASpec(serviceAccountName string, deviceClassNames []string,
	extraEnv []map[string]interface{}) map[string]interface{} {
	spec := map[string]interface{}{
		"driverName":         kmmparams.DRADriverName,
		"serviceAccountName": serviceAccountName,
		"container":          DRAContainer(extraEnv),
	}

	if len(deviceClassNames) > 0 {
		classes := make([]interface{}, len(deviceClassNames))
		for i, name := range deviceClassNames {
			classes[i] = map[string]interface{}{"name": name}
		}

		spec["deviceClasses"] = classes
	}

	return spec
}

// DRASpecWithCELSelector returns DRASpec with a CEL selector on each DeviceClass matching kmmparams.DRADriverName.
func DRASpecWithCELSelector(serviceAccountName string, deviceClassNames []string,
	extraEnv []map[string]interface{}) map[string]interface{} {
	spec := DRASpec(serviceAccountName, deviceClassNames, extraEnv)

	classes, ok := spec["deviceClasses"].([]interface{})
	if !ok {
		return spec
	}

	expression := fmt.Sprintf("device.driver == '%s'", kmmparams.DRADriverName)

	for i, class := range classes {
		deviceClass, ok := class.(map[string]interface{})
		if !ok {
			continue
		}

		deviceClass["selectors"] = []interface{}{
			map[string]interface{}{
				"cel": map[string]interface{}{
					"expression": expression,
				},
			},
		}
		classes[i] = deviceClass
	}

	spec["deviceClasses"] = classes

	return spec
}

// UnstructuredModule returns an unstructured Module object with the given spec.
func UnstructuredModule(name, nsname string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kmm.sigs.x-k8s.io/v1beta1",
			"kind":       "Module",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": nsname,
			},
			"spec": spec,
		},
	}
}

// ModuleSpec builds a ModuleSpec with a regex kernel mapping and module loader container.
func ModuleSpec(apiClient *clients.Settings, name, nsname, kmodName, image string,
	nodeSelector map[string]string) (moduleV1Beta1.ModuleSpec, error) {
	kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
		WithContainerImage(image).
		BuildKernelMappingConfig()
	if err != nil {
		return moduleV1Beta1.ModuleSpec{}, err
	}

	moduleLoaderCfg, err := kmm.NewModLoaderContainerBuilder(kmodName).
		WithKernelMapping(kernelMapping).
		BuildModuleLoaderContainerCfg()
	if err != nil {
		return moduleV1Beta1.ModuleSpec{}, err
	}

	builder := kmm.NewModuleBuilder(apiClient, name, nsname)
	if builder == nil {
		return moduleV1Beta1.ModuleSpec{}, fmt.Errorf("failed to create module builder")
	}

	return builder.
		WithNodeSelector(nodeSelector).
		WithModuleLoaderContainer(moduleLoaderCfg).
		BuildModuleSpec()
}
