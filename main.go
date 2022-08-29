package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	gcpapi "github.com/ahanafy/kubesync/internal"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

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

	var projectID = viper.GetString("GCP_PROJECT_ID")

	creds, err := ioutil.ReadFile(viper.GetString("GOOGLE_APPLICATION_CREDENTIALS"))

	if err != nil {
		fmt.Println(err)
	}

	g := new(gcpapi.GCPCreds)
	g.Creds = creds

	// Get all GCP secrets phase
	secretslist, errlist := g.ListSecrets(fmt.Sprintf("projects/%s", projectID))
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

	// Kubernetes access phase

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

	labelSelector := viper.GetString("LABEL")

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
		var k8sObject *corev1.Secret
		var k []byte = nil
		// retrieve the Secret
		item := event.Object.(*corev1.Secret)

		switch event.Type {

		// when a secret is deleted...
		case watch.Deleted:
			fmt.Printf("- '%s' %v ...Deleted\n", item.GetName(), event.Type)
		// when a secret is added...
		case watch.Added:
			fmt.Println(" ...Added!")
			k8sObject = changeEvent(item, event, rc)

			k, err = marshalK8s(k8sObject)
			if err != nil {
				fmt.Println(err)
			}

		// when a secret is modified...
		case watch.Modified:
			fmt.Println("...Modified!")
			k8sObject = changeEvent(item, event, rc)
			k, err = marshalK8s(k8sObject)
			if err != nil {
				fmt.Println(err)
			}
		}
		if k != nil {
			err = g.WriteSecret(projectID, k8sObject.Name, k)
			if err != nil {
				fmt.Println(err)
				fmt.Printf("Coudln't create: %s\n", k8sObject.Name)
			} else {
				fmt.Printf("Created: %s\n", k8sObject.Name)
			}
		}

	}
}

func marshalK8s(k8sObject *corev1.Secret) ([]byte, error) {
	k, err := json.Marshal(k8sObject)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(k))
	return k, err
}

func changeEvent(item *corev1.Secret, event watch.Event, rc *rest.RESTClient) *corev1.Secret {
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

	return secret
}
