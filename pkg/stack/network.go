package stack

import (
	"context"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// ensureNetwork creates the Docker network if it doesn't exist.
func ensureNetwork(ctx context.Context, networkName string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()

	// Check if network already exists
	networks, err := cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return err
	}

	for _, n := range networks {
		if n.Name == networkName {
			return nil // Already exists
		}
	}

	// Create the network
	_, err = cli.NetworkCreate(ctx, networkName, network.CreateOptions{
		Driver: "bridge",
	})
	return err
}
