package providers

import (
	"errors"
	"fmt"
)

type awsBedrockHTTPError struct {
	Service    string
	StatusCode int
	Endpoint   string
}

func (e *awsBedrockHTTPError) Error() string {
	return fmt.Sprintf("aws-bedrock %s %s: HTTP %d", e.Service, e.Endpoint, e.StatusCode)
}

func wrapAWSBedrockPermissionHint(err error, service string) error {
	var he *awsBedrockHTTPError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case 400, 401, 403, 404:
			return fmt.Errorf("%w: AWS %s request failed. Check AWS_ACCESS_KEY_ID, "+
				"AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN if used, IAM permissions, and region settings",
				err, service)
		}
	}
	return err
}
