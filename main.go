package main

import (
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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	// Create a factory object that we can say "hey, I need to watch this resource"
	// and it will give us back an informer for it
	f := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dc, 0, v1.NamespaceAll, nil)
	// Retrieve a "GroupVersionResource" type that we need when generating our informer from our dynamic factory
	gvr, _ := schema.ParseResourceArg(resource)
	// Finally, create our informer for deployments!
	i := f.ForResource(*gvr)
	stopCh := make(chan struct{})
	go startWatching(stopCh, i.Informer(), projectID, logger, g)
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Kill, os.Interrupt)
	<-sigCh
	close(stopCh)

}

func marshalK8s(k8sObject *corev1.Secret) ([]byte, error) {
	k, err := json.Marshal(k8sObject)
	if err != nil {
		fmt.Println(err)
	}

	return k, err
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
func startWatching(stopCh <-chan struct{}, s cache.SharedIndexInformer, projectID string, logger *zap.Logger, g *gcpapi.GCPCreds) {

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
			writeIt(u, projectID, g)

		},
		UpdateFunc: func(oldObj, obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			sugar.Infow("received update event!",
				"name", u.GetName(),
				"namespace", u.GetNamespace(),
				"labels", u.GetLabels(),
			)
			writeIt(u, projectID, g)
		},
		DeleteFunc: func(obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			sugar.Infow("received delete event!",
				"name", u.GetName(),
				"namespace", u.GetNamespace(),
				"labels", u.GetLabels(),
				"type", u.GetKind(),
			)
		},
	}

	s.AddEventHandler(handlers)
	s.Run(stopCh)
}

func writeIt(k *unstructured.Unstructured, projectID string, g *gcpapi.GCPCreds) {
	if k != nil && k.GetKind() == "Secret" {
		// Unstructured -> Typed
		var tSecret corev1.Secret
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(k.Object, &tSecret)
		if err != nil {
			panic(err.Error())
		}
		ts, err := marshalK8s(&tSecret)
		if err != nil {
			panic(err.Error())
		}
		err = g.WriteSecret(projectID, k.GetName(), ts)
		if err != nil {
			fmt.Println(err)
			fmt.Printf("Couldn't create: %s\n", k.GetName())
		} else {
			fmt.Printf("Created: %s\n", k.GetName())
		}
	}
}
