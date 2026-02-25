package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestParseRestoreHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"empty", "", ""},
		{"in progress", `ongoing-request="true"`, "in-progress"},
		{"available", `ongoing-request="false", expiry-date="Fri, 23 Dec 2022 00:00:00 GMT"`, "available"},
		{"available no expiry", `ongoing-request="false"`, "available"},
		{"unrecognized", "something-else", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRestoreHeader(tt.header)
			if got != tt.want {
				t.Errorf("ParseRestoreHeader(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestValidateTier(t *testing.T) {
	tests := []struct {
		name         string
		storageClass string
		tier         types.Tier
		wantErr      bool
	}{
		{"deep archive expedited blocked", "DEEP_ARCHIVE", types.TierExpedited, true},
		{"deep archive standard ok", "DEEP_ARCHIVE", types.TierStandard, false},
		{"deep archive bulk ok", "DEEP_ARCHIVE", types.TierBulk, false},
		{"glacier expedited ok", "GLACIER", types.TierExpedited, false},
		{"glacier standard ok", "GLACIER", types.TierStandard, false},
		{"standard expedited ok", "STANDARD", types.TierExpedited, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTier(tt.storageClass, tt.tier)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTier(%q, %q) error = %v, wantErr %v", tt.storageClass, tt.tier, err, tt.wantErr)
			}
		})
	}
}

func TestParseTier(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    types.Tier
		wantErr bool
	}{
		{"standard lowercase", "standard", types.TierStandard, false},
		{"standard titlecase", "Standard", types.TierStandard, false},
		{"standard uppercase", "STANDARD", types.TierStandard, false},
		{"bulk", "bulk", types.TierBulk, false},
		{"bulk titlecase", "Bulk", types.TierBulk, false},
		{"expedited", "expedited", types.TierExpedited, false},
		{"expedited titlecase", "Expedited", types.TierExpedited, false},
		{"invalid", "fast", "", true},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTier(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTier(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseTier(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
