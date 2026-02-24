package aws

import (
	"context"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ParseRestoreHeader parses the x-amz-restore header value.
// Examples:
//
//	ongoing-request="true"                                    -> "in-progress"
//	ongoing-request="false", expiry-date="Fri, 23 Dec 2012..." -> "available"
//	""                                                        -> ""
func ParseRestoreHeader(header string) string {
	if header == "" {
		return ""
	}
	if strings.Contains(header, `ongoing-request="true"`) {
		return "in-progress"
	}
	if strings.Contains(header, `ongoing-request="false"`) {
		return "available"
	}
	return ""
}

// ValidateTier checks that the requested tier is valid for the given storage class.
// Deep Archive does not support Expedited retrieval.
func ValidateTier(storageClass string, tier types.Tier) error {
	if storageClass == "DEEP_ARCHIVE" && tier == types.TierExpedited {
		return fmt.Errorf("Expedited retrieval is not available for DEEP_ARCHIVE storage class; use Standard or Bulk")
	}
	return nil
}

func (c *Client) RestoreObject(ctx context.Context, input RestoreInput) error {
	_, err := c.svc.RestoreObject(ctx, &s3.RestoreObjectInput{
		Bucket: awssdk.String(input.Bucket),
		Key:    awssdk.String(input.Key),
		RestoreRequest: &types.RestoreRequest{
			Days: awssdk.Int32(input.Days),
			GlacierJobParameters: &types.GlacierJobParameters{
				Tier: input.Tier,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("restoring object %s: %w", input.Key, err)
	}
	return nil
}

func ParseTier(s string) (types.Tier, error) {
	switch strings.ToLower(s) {
	case "standard":
		return types.TierStandard, nil
	case "bulk":
		return types.TierBulk, nil
	case "expedited":
		return types.TierExpedited, nil
	default:
		return "", fmt.Errorf("invalid tier %q; valid options: Standard, Bulk, Expedited", s)
	}
}
