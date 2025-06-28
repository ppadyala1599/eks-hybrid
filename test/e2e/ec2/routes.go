package ec2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// CreateRouteForCIDRToInstance creates a route entry in the table routing a cidr to an instance.
func CreateRouteForCIDRToInstance(ctx context.Context, client *ec2.Client, routeTableID, cidr, instanceID string) error {
	_, err := client.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         aws.String(routeTableID),
		DestinationCidrBlock: aws.String(cidr),
		InstanceId:           aws.String(instanceID),
	}, func(o *ec2.Options) {
		// adding routes occasionally fails with instance state "unknown-running"
		// retrying this error to allow for any async route table/instance state changes to complete
		o.Retryer = retry.AddWithErrorCodes(o.Retryer, "IncorrectInstanceState")
	})
	if err != nil {
		return fmt.Errorf("could not create route to instance %s for dst CIDR %s: %w", instanceID, cidr, err)
	}
	return nil
}
