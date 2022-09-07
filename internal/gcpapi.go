package gcpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"reflect"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/googleapis/gax-go/v2/apierror"
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type GCPCreds struct {
	Creds []byte
	Rc    *rest.RESTClient
	Kc    *kubernetes.Clientset
}

type Secret struct {
	Name    string
	Path    string
	Payload []byte
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
			otelzap.S().
				Ctx(context.TODO()).
				Errorw("Client error while connecting!",
					err,
				)
			errors = append(errors, err)

			if e, ok := err.(*apierror.APIError); ok {
				if e.GRPCStatus().Code() == 7 {
					os.Exit(1)
				}
				return secrets, errors
			}
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
		otelzap.S().Ctx(ctx).Errorw("CreateSecret error", err)
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

	// Collect all backed up values
	valueList := g.ReconcileSecrets(projectID)

	for _, secret := range valueList {
		// if the `secretName` from the cluster is found in the list of remote secret[secretName] then...
		if s, found := secret[secretName]; found {

			if s.Name == secretName {
				delete(secret, secretName)
			}
		}
	}
	err := g.idempotentCreateRemoteSecret(projectID, secretName, payload)
	// loop over elements of slice
	for _, secret := range valueList {

		for _, v := range secret {
			// fmt.Println(k, "value is", v)
			var secret *corev1.Secret
			if err := json.Unmarshal([]byte(v.Payload), &secret); err != nil {
				panic(err)
			}

			secretsClient := g.Kc.CoreV1().Secrets(secret.Namespace)
			_, err := secretsClient.Get(context.TODO(), secret.Name, metav1.GetOptions{})
			if err != nil {
				g.ApplyK8s(v, "create")
			}

		}

	}
	return err
}

func (g GCPCreds) ReconcileSecrets(projectID string) []map[string]Secret {
	items := make([]map[string]Secret, 0)

	// Build remote secrets list
	// Get all GCP secrets phase
	secretslist, errlist := g.ListSecrets(fmt.Sprintf("projects/%s", projectID))
	for _, err := range errlist {
		if err != nil {
			otelzap.S().Ctx(context.TODO()).Errorw("Error building secrets list!",
				"error", err,
			)
		}

	}

	for _, secret := range secretslist {
		item := make(map[string]Secret)
		result, err := g.AccessSecretVersion(fmt.Sprintf("%s/versions/latest", secret))
		if err != nil {
			fmt.Println(err)
		} else {
			item[path.Base(secret)] = Secret{
				Name:    path.Base(secret),
				Path:    secret,
				Payload: result,
			}
			items = append(items, item)
		}

	}
	return items
}

func (g GCPCreds) ApplyK8s(sec Secret, verb string) {
	
	var secret *corev1.Secret
	if err := json.Unmarshal(sec.Payload, &secret); err != nil {
		panic(err)
	}
	secret.UID = ""
	secret.ResourceVersion = ""

	secretsClient := g.Kc.CoreV1().Secrets(secret.Namespace)
	if verb == "update" {
		// Update Secret
		fmt.Println("Updating secret...")
		results, err := secretsClient.Update(context.TODO(), secret, metav1.UpdateOptions{})
		if err != nil {
			panic(err)
		}
		fmt.Printf("Updated secret %q.\n", results.GetObjectMeta().GetName())
	} else {
		// Create Secret
		fmt.Println("Creating secret...")
		results, err := secretsClient.Create(context.TODO(), secret, metav1.CreateOptions{})
		if err != nil {
			panic(err)
		}
		fmt.Printf("Created secret %q.\n", results.GetObjectMeta().GetName())
	}

}

func (g GCPCreds) idempotentCreateRemoteSecret(projectID string, secretName string, payload []byte) error {
	var addVersion bool = true
	// Create GCP secret phase
	gcpSecretName, err := g.CreateSecret(fmt.Sprintf("projects/%s", projectID), secretName)

	// Check for access error, or it could already exist
	if err != nil {

		e, ok := err.(*apierror.APIError)
		if !ok {
			otelzap.S().Ctx(context.TODO()).Errorw("Unable to make API request", err)
			return err
		}
		// Check if secret already exists
		if e.GRPCStatus().Code() == 6 {
			gcpSecretName = fmt.Sprintf("projects/%s/secrets/%s", projectID, secretName)

			var originalPayload corev1.Secret
			if err := json.Unmarshal(payload, &originalPayload); err != nil {
				panic(err)
			}

			gcpPayloadBytes, err := g.AccessSecretVersion(fmt.Sprintf("%s/versions/latest", gcpSecretName))
			if err != nil {
				return err
			}
			var gcpPayload corev1.Secret
			if err := json.Unmarshal(gcpPayloadBytes, &gcpPayload); err != nil {
				panic(err)
			}

			if reflect.DeepEqual(&originalPayload, &gcpPayload) {
				addVersion = false
			}

		} else {
			return e
		}
	}

	if addVersion {
		// Input data.
		versionResponse, err := g.AddSecretVersion(gcpSecretName, payload)
		if err != nil {
			return err
		}
		fmt.Println(versionResponse)
	}
	return nil
}
