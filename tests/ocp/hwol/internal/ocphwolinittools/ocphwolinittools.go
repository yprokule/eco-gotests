package ocphwolinittools

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	hwolconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolconfig"
)

var (
	// APIClient provides API access to cluster.
	APIClient *clients.Settings
	// HwolOcpConfig provides access to HWOL configuration parameters.
	HwolOcpConfig *hwolconfig.HwolOcpConfig
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
func init() {
	HwolOcpConfig = hwolconfig.NewHwolOcpConfig()
	APIClient = inittools.APIClient
}
