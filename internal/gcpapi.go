package gcpapi

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/googleapis/gax-go/v2/apierror"
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

// CreateSecret creates a new secret in the Google Cloud Manager top-
// level directory, specified as `parent`, using the `secretID` provided
// as the name.
func (g GCPCreds) CreateSecret(parent string, secretID string) (string, error) {

	// Create the client.
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx, option.WithCredentialsJSON(g.Creds))
	if err != nil {
		// The most likely causes of the error are:
		//     1 - google application creds failed
		//     2 - secret already exists
		return "", fmt.Errorf("failed to create secretmanager client: %v", err)
	}
	defer client.Close()

	// Build the request.
	req := &secretmanagerpb.CreateSecretRequest{
		Parent:   parent,
		SecretId: secretID,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	}

	// Call the API.
	result, err := client.CreateSecret(ctx, req)
	if err != nil {

		return "", err
	}
	fmt.Printf("created secret: %s\n", result.Name)
	return result.Name, nil
}

// AddSecretVersion adds a new secret version to the given secret path with the
// provided payload.
func (g GCPCreds) AddSecretVersion(path string, payload []byte) (string, error) {

	// Create the client.
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx, option.WithCredentialsJSON(g.Creds))
	if err != nil {
		return "", fmt.Errorf("failed to create secretmanager client: %v", err)
	}
	defer client.Close()

	// Build the request.
	req := &secretmanagerpb.AddSecretVersionRequest{
		Parent: path,
		Payload: &secretmanagerpb.SecretPayload{
			Data: payload,
		},
	}

	// Call the API.
	result, err := client.AddSecretVersion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to add secret version: %v", err)
	}

	fmt.Printf("added secret version: %s\n", result.Name)
	return result.Name, nil
}

func (g GCPCreds) WriteSecret(projectID string, secretName string, payload []byte) error {

	// Create GCP secret phase
	gcpSecretName, err := g.CreateSecret(fmt.Sprintf("projects/%s", projectID), secretName)

	// Check for access error, or it could already exist
	if err != nil {
		// Check if secret already exists
		if err.(*apierror.APIError).GRPCStatus().Code() == 6 {
			fmt.Println("Already exists")
			gcpSecretName = fmt.Sprintf("projects/%s/secrets/%s", projectID, secretName)
		} else {
			return err
		}
	}

	// Input data.
	versionResponse, err := g.AddSecretVersion(gcpSecretName, payload)
	if err != nil {
		return err
	}
	fmt.Println(versionResponse)
	return nil
}
