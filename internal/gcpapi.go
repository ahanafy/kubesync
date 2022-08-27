package gcpapi

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

type GCPCreds struct {
	Creds []byte
}

// AccessSecretVersion returns the payload for the given secret version if one
// exists.
func (g GCPCreds) AccessSecretVersion(version string) ([]byte, error) {

	// Create the client.
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx, option.WithCredentialsJSON(g.Creds))
	if err != nil {
		return nil, fmt.Errorf("failed to create secretmanager client: %v", err)
	}
	defer client.Close()

	// Build the request.
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: version,
	}

	// Call the API.
	result, err := client.AccessSecretVersion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to access secret version: %v", err)
	}

	fmt.Printf("retrieved payload for: %s\n", result.Name)
	return result.Payload.Data, nil
}

// ListSecrets retrieves the names of all secrets in the project,
// given the `parent` (string).
func (g GCPCreds) ListSecrets(parent string) (secrets []string, errors []error) {

	// Create the client.
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx, option.WithCredentialsJSON(g.Creds))
	if err != nil {
		return secrets, append(errors, err)
	}
	defer client.Close()

	// Build the request.
	req := &secretmanagerpb.ListSecretsRequest{
		Parent: parent,
	}

	// Call the API.
	it := client.ListSecrets(ctx, req)

	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}

		if err != nil {
			errors = append(errors, err)
			secrets = append(secrets, "")
			continue
		}
		secrets = append(secrets, resp.Name)
		errors = append(errors, nil)
	}
	return secrets, errors
}
