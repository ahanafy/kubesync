package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/spf13/viper"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type sealedSecretKeyPair struct {
	name       string
	namespace  string
	publicKey  string
	privateKey string
	labels     map[string]string
}

type GCPCreds struct {
	creds []byte
}

func main() {
	env := "./config/config.yaml"
	if strings.EqualFold(os.Getenv("DEBUG"), "TRUE") {
		env = "./config/config.local.yaml"
	}
	viper.SetConfigFile(env)
	viper.SetConfigType("yaml")
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Println(err)
	}

	creds, err := ioutil.ReadFile(viper.GetString("GOOGLE_APPLICATION_CREDENTIALS"))

	if err != nil {
		fmt.Println(err)
	}
	g := GCPCreds{creds}

	fmt.Println("ALL")
	secretslist, errlist := g.ListSecrets(fmt.Sprintf("projects/%s", viper.GetString("GCP_PROJECT_ID")))
	for _, err := range errlist {
		if err != nil {
			fmt.Println(err)
		}
	}

	for _, secret := range secretslist {
		
		result, err := g.AccessSecretVersion(fmt.Sprintf("%s/versions/latest", secret))
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(string(result))
	}

	// Using the default configuration rules get the info
	// to connect to the Kubernetes cluster
	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)

	// create the Config object
	cfg, err := configLoader.ClientConfig()

	if err != nil {
		panic(err)
	}

	// we want to use the core API (secrets lives here)
	cfg.APIPath = "/api"
	cfg.GroupVersion = &corev1.SchemeGroupVersion
	cfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	// create a RESTClient
	rc, err := rest.RESTClientFor(cfg)
	if err != nil {
		panic(err.Error())
	}

	// utility function to create a int64 pointer
	i64Ptr := func(i int64) *int64 { return &i }

	value := "active"
	labelSelector := fmt.Sprintf("sealedsecrets.bitnami.com/sealed-secrets-key=%s", value)

	opts := metav1.ListOptions{
		TimeoutSeconds: i64Ptr(120),
		LabelSelector:  labelSelector,
		Watch:          true,
	}

	// attempts to begin watching the secrets
	// returns a `watch.Interface`, or an error
	watcher, err := rc.Get().Resource("secrets").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(time.Duration(*opts.TimeoutSeconds)).
		Watch(context.TODO())
	if err != nil {
		panic(err)
	}

	// here we iterate all the events streamed by the watch.Interface
	for event := range watcher.ResultChan() {
		// retrieve the Secret
		item := event.Object.(*corev1.Secret)

		switch event.Type {

		// when a secret is deleted...
		case watch.Deleted:
			// let's say hello!
			fmt.Printf("- '%s' %v ...Deleted\n", item.GetName(), event.Type)

		// when a secret is added...
		case watch.Added:
			fmt.Println(changeEvent(item, event, rc))

			fmt.Println(" ...Added!")
		case watch.Modified:
			fmt.Println(changeEvent(item, event, rc))
			fmt.Println("...Modified!")
		}
	}
}

func changeEvent(item *corev1.Secret, event watch.Event, rc *rest.RESTClient) sealedSecretKeyPair {
	fmt.Printf("+ '%s' %v  ", item.GetName(), event.Type)
	secret := &corev1.Secret{}
	err := rc.Get().Resource(("secrets")).
		Namespace("default").
		Name(item.Name).
		Do(context.TODO()).
		Into(secret)

	if err != nil {
		fmt.Println(err)
	}

	sobject := sealedSecretKeyPair{
		name:       secret.Name,
		namespace:  secret.Namespace,
		publicKey:  string(secret.Data["tls.crt"]),
		privateKey: string(secret.Data["tls.key"]),
		labels:     secret.Labels,
	}

	return sobject
}

// AccessSecretVersion returns the payload for the given secret version if one
// exists.
func (g GCPCreds) AccessSecretVersion(version string) ([]byte, error) {

	// Create the client.
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx, option.WithCredentialsJSON(g.creds))
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
	client, err := secretmanager.NewClient(ctx, option.WithCredentialsJSON(g.creds))
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
