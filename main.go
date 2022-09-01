package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	gcpapi "github.com/ahanafy/kubesync/internal"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// TODO - Get from config.yaml
var resource = "secrets.v1."

func main() {
	logger, _ := zap.NewProduction()
	defer func() {
		_ = logger.Sync()
	}()
	sugar := logger.Sugar()

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
	logger.Info(projectID)
	creds, err := ioutil.ReadFile(viper.GetString("GOOGLE_APPLICATION_CREDENTIALS"))

	if err != nil {
		fmt.Println(err)
	}

	g := new(gcpapi.GCPCreds)
	g.Creds = creds

	// Kubernetes access phase

	ccfg, err := restConfig()
	if err != nil {
		sugar.Fatal("could not get config",
			zap.Error(err),
		)
	}

	// Grab a dynamic interface that we can create informers from
	dc, err := dynamic.NewForConfig(ccfg)
	if err != nil {
		sugar.Fatal("could not generate dynamic client for config",
			zap.Error(err),
		)
	}

	g.Kc, err = kubernetes.NewForConfig(ccfg)
	if err != nil {
		panic(err)
	}

	// create a RESTClient
	// rc, err := rest.RESTClientFor(ccfg)
	// if err != nil {
	// 	panic(err.Error())
	// }
	// g.Rc = rc

	// Create a factory object that we can say "hey, I need to watch this resource"
	// and it will give us back an informer for it
	f := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dc, 0, v1.NamespaceAll, nil)
	// Retrieve a "GroupVersionResource" type that we need when generating our informer from our dynamic factory
	gvr, _ := schema.ParseResourceArg(resource)
	// Finally, create our informer for deployments!
	i := f.ForResource(*gvr)
	stopCh := make(chan struct{})
	go startWatching(stopCh, i.Informer(), logger)
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Kill, os.Interrupt)
	<-sigCh
	close(stopCh)

	// here we iterate all the events streamed by the watch.Interface
	// for event := range watcher.ResultChan() {
	// 	var k8sObject *corev1.Secret
	// 	var k []byte = nil
	// 	// retrieve the Secret
	// 	item := event.Object.(*corev1.Secret)

	// 	switch event.Type {

	// 	// when a secret is deleted...
	// 	case watch.Deleted:
	// 		fmt.Printf("- '%s' %v ...Deleted\n", item.GetName(), event.Type)
	// 	// when a secret is added...
	// 	case watch.Added:
	// 		fmt.Println(" ...Added!")
	// 		k8sObject = changeEvent(item, event, rc)

	// 		k, err = marshalK8s(k8sObject)
	// 		if err != nil {
	// 			fmt.Println(err)
	// 		}
	// 	// when a secret is modified...
	// 	case watch.Modified:
	// 		fmt.Println("...Modified!")
	// 		k8sObject = changeEvent(item, event, rc)
	// 		k, err = marshalK8s(k8sObject)
	// 		if err != nil {
	// 			fmt.Println(err)
	// 		}
	// 	}
	// 	if k != nil {
	// 		err = g.WriteSecret(projectID, k8sObject.Name, k)
	// 		if err != nil {
	// 			fmt.Println(err)
	// 			fmt.Printf("Couldn't create: %s\n", k8sObject.Name)
	// 		} else {
	// 			fmt.Printf("Created: %s\n", k8sObject.Name)
	// 		}
	// 	}

	// }
}

func marshalK8s(k8sObject *corev1.Secret) ([]byte, error) {
	k, err := json.Marshal(k8sObject)
	if err != nil {
		fmt.Println(err)
	}

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

func restConfig() (*rest.Config, error) {
	kubeCfg, err := rest.InClusterConfig()
	var kubeconfig *string
	if err != nil {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}
		flag.Parse()

		kubeCfg, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err)
		}
	}

	if err != nil {
		return nil, err
	}
	return kubeCfg, nil
}
func startWatching(stopCh <-chan struct{}, s cache.SharedIndexInformer, logger *zap.Logger) {

	defer func() {
		_ = logger.Sync()
	}()
	sugar := logger.Sugar()
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			sugar.Infow("received add event!",
				"name", u.GetName(),
				"namespace", u.GetNamespace(),
				"labels", u.GetLabels(),
			)
		},
		UpdateFunc: func(oldObj, obj interface{}) {
			sugar.Info("received update event!")
		},
		DeleteFunc: func(obj interface{}) {
			sugar.Info("received delete event!")
		},
	}
	s.AddEventHandler(handlers)
	s.Run(stopCh)
}
