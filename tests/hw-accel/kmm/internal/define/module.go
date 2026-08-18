package define

import (
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
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

// DRASpec builds the unstructured dra portion of a Module spec.
// deviceClassNames may be nil for modules with no deviceClasses.
// extraEnv is appended after the default DRIVER_NAME env var.
func DRASpec(serviceAccountName string, deviceClassNames []string,
	extraEnv []map[string]interface{}) map[string]interface{} {
	env := []interface{}{
		map[string]interface{}{
			"name":  "DRIVER_NAME",
			"value": kmmparams.DRADriverName,
		},
	}

	for _, e := range extraEnv {
		env = append(env, e)
	}

	spec := map[string]interface{}{
		"driverName":         kmmparams.DRADriverName,
		"serviceAccountName": serviceAccountName,
		"container": map[string]interface{}{
			"image":   kmmparams.DRADriverImage,
			"command": []interface{}{"dra-example-kubeletplugin"},
			"env":     env,
		},
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
