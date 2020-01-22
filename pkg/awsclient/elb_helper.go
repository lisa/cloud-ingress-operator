package awsclient

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elb"
)

// CreateClassicELB creates a classic ELB in Amazon, as in for management API endpoint.
// inputs are the name of the ELB, the availability zone(s) and subnet(s) the
// ELB should attend, as well as the listener port.
// The port is used for the instance port and load balancer port
// Return is the (FQDN) DNS name from Amazon, and error, if any.
func (c *awsClient) CreateClassicELB(elbName string, subnets []string, listenerPort int64) (string, error) {
	fmt.Printf("  * CreateClassicELB(%s,%s,%d)\n", elbName, subnets, listenerPort)
	i := &elb.CreateLoadBalancerInput{
		LoadBalancerName: aws.String(elbName),
		Subnets:          aws.StringSlice(subnets),
		//AvailabilityZones: aws.StringSlice(availabilityZones),
		Listeners: []*elb.Listener{
			{
				InstancePort:     aws.Int64(listenerPort),
				InstanceProtocol: aws.String("tcp"),
				Protocol:         aws.String("tcp"),
				LoadBalancerPort: aws.Int64(listenerPort),
			},
		},
	}
	o, err := c.CreateLoadBalancer(i)
	if err != nil {
		return "", err
	}
	fmt.Printf("    * Adding health check (HTTP:6443/)\n")
	err = c.addHealthCheck(elbName, "HTTP", "/", 6443)
	if err != nil {
		return "", err
	}
	return *o.DNSName, nil
}

// SetLoadBalancerPrivate sets a load balancer private by removing its
// listeners (port 6443/TCP)
func (c *awsClient) SetLoadBalancerPrivate(elbName string) error {
	return c.removeListenersFromELB(elbName)
}

// SetLoadBalancerPublic will set the specified load balancer public by
// re-adding the 6443/TCP -> 6443/TCP listener. Any instances (still)
// attached to the load balancer will begin to receive traffic.
func (c *awsClient) SetLoadBalancerPublic(elbName string, listenerPort int64) error {
	l := []*elb.Listener{
		{
			InstancePort:     aws.Int64(listenerPort),
			InstanceProtocol: aws.String("tcp"),
			Protocol:         aws.String("tcp"),
			LoadBalancerPort: aws.Int64(listenerPort),
		},
	}
	return c.addListenersToELB(elbName, l)
}

// removeListenersFromELB will remove the 6443/TCP -> 6443/TCP listener from
// the specified ELB. This is useful when the "ext" ELB is to be no longer
// publicly accessible
func (c *awsClient) removeListenersFromELB(elbName string) error {
	i := &elb.DeleteLoadBalancerListenersInput{
		LoadBalancerName:  aws.String(elbName),
		LoadBalancerPorts: aws.Int64Slice([]int64{6443}),
	}
	_, err := c.DeleteLoadBalancerListeners(i)
	return err
}

// addListenersToELB will add the +listeners+ to the specified ELB. This is
// useful for when the "ext" ELB is to be publicly accessible. See also
// removeListenersFromELB.
// Note: This will likely always want to be given 6443/tcp -> 6443/tcp for
// the kube-api
func (c *awsClient) addListenersToELB(elbName string, listeners []*elb.Listener) error {
	i := &elb.CreateLoadBalancerListenersInput{
		Listeners:        listeners,
		LoadBalancerName: aws.String(elbName),
	}
	_, err := c.CreateLoadBalancerListeners(i)
	return err
}

// AddLoadBalancerInstances will attach +instanceIds+ to +elbName+
// so that they begin to receive traffic. Note that this takes an amount of
// time to return. This is also additive (but idempotent - TODO: Validate this).
// Note that the recommended steps:
// 1. stop the instance,
// 2. deregister the instance,
// 3. start the instance,
// 4. and then register the instance.
func (c *awsClient) AddLoadBalancerInstances(elbName string, instanceIds []string) error {
	instances := make([]*elb.Instance, 0)
	for _, instance := range instanceIds {
		instances = append(instances, &elb.Instance{InstanceId: aws.String(instance)})
	}
	i := &elb.RegisterInstancesWithLoadBalancerInput{
		Instances:        instances,
		LoadBalancerName: aws.String(elbName),
	}
	_, err := c.RegisterInstancesWithLoadBalancer(i)
	return err
}

// RemoveInstancesFromLoadBalancer removes +instanceIds+ from +elbName+, eg when an Node is deleted.
func (c *awsClient) RemoveInstancesFromLoadBalancer(elbName string, instanceIds []string) error {
	instances := make([]*elb.Instance, 0)
	for _, instance := range instanceIds {
		instances = append(instances, &elb.Instance{InstanceId: aws.String(instance)})
	}
	i := &elb.DeregisterInstancesFromLoadBalancerInput{
		Instances:        instances,
		LoadBalancerName: aws.String(elbName),
	}
	_, err := c.DeregisterInstancesWithLoadBalancer(i)
	return err
}

// DoesELBExist checks for the existence of an ELB by name. If there's an AWS
// error it is returned.
func (c *awsClient) DoesELBExist(elbName string) (bool, string, error) {
	i := &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(elbName)},
	}
	res, err := c.DescribeLoadBalancers(i)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elb.ErrCodeAccessPointNotFoundException:
				return false, "", nil
			default:
				return false, "", err
			}
		}
	}
	return true, *res.LoadBalancerDescriptions[0].DNSName, nil
}

func (c *awsClient) addHealthCheck(loadBalancerName, protocol, path string, port int64) error {
	i := &elb.ConfigureHealthCheckInput{
		HealthCheck: &elb.HealthCheck{
			HealthyThreshold:   aws.Int64(2),
			Interval:           aws.Int64(30),
			Target:             aws.String(fmt.Sprintf("%s:%d%s", protocol, port, path)),
			Timeout:            aws.Int64(3),
			UnhealthyThreshold: aws.Int64(2),
		},
		LoadBalancerName: aws.String(loadBalancerName),
	}
	_, err := c.ConfigureHealthCheck(i)
	return err
}
