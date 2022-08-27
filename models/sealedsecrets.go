package kubernetes

type Secret struct {
	Name      string
	Namespace string
	Labels    map[string]string
}

type TLSSecret struct {
	Secret     Secret
	PublicKey  string
	PrivateKey string
}
