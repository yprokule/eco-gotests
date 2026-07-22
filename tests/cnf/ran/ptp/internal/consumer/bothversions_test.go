//go:build unit_test

package consumer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveEventAPIVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		ptpVersion        string
		configAPIVersion  string
		wantAPIVersion    ptpEventAPIVersion
		wantErrorContains string
	}{
		{
			name:             "PTP 4.14 without apiVersion field defaults to v1",
			ptpVersion:       "4.14.0",
			configAPIVersion: "",
			wantAPIVersion:   eventAPIVersionV1,
		},
		{
			name:              "PTP 4.16 with unset apiVersion returns error",
			ptpVersion:        "4.16.0",
			configAPIVersion:  "",
			wantErrorContains: "unknown event API version  in PTP operator config",
		},
		{
			name:             "PTP 4.16 with explicit v1 uses v1",
			ptpVersion:       "4.16.0",
			configAPIVersion: "1.0",
			wantAPIVersion:   eventAPIVersionV1,
		},
		{
			name:             "PTP 4.18 with explicit v2 uses v2",
			ptpVersion:       "4.18.0",
			configAPIVersion: "2.0",
			wantAPIVersion:   eventAPIVersionV2,
		},
		{
			name:             "PTP 4.19 always uses v2 even when config says v1",
			ptpVersion:       "4.19.0",
			configAPIVersion: "1.0",
			wantAPIVersion:   eventAPIVersionV2,
		},
		{
			name:              "unknown apiVersion returns error",
			ptpVersion:        "4.17.0",
			configAPIVersion:  "3.0",
			wantErrorContains: "unknown event API version 3.0 in PTP operator config",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			gotAPIVersion, err := resolveEventAPIVersion(testCase.ptpVersion, testCase.configAPIVersion)
			if testCase.wantErrorContains != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), testCase.wantErrorContains)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, testCase.wantAPIVersion, gotAPIVersion)
		})
	}
}
