package acme

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/traefik/traefik/v2/pkg/log"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// LabelResolver is the key of the Kubernetes label where we store the secret's resolver name.
const LabelResolver = "traefik.ingress.kubernetes.io/resolver"

// LabelACMEStorage is the key of the Kubernetes label that marks a secret as stored.
const LabelACMEStorage = "traefik.ingress.kubernetes.io/acme-storage"

// KubernetesSecretStore stores ACME account and certificates Kubernetes secrets.
// Each domain is stored as a separate value in the secret.
// All secrets managed by this store will get the label
// `traefik.ingress.kubernetes.io/acme-storage=true`.
type KubernetesSecretStore struct {
	namespace  string
	secretName string
	client     kubernetes.Interface

	lock       sync.Mutex
	storedData map[string]*StoredData
}

// NewKubernetesSecretStore creates a new KubernetesSecretStore instance.
func NewKubernetesSecretStore(storage *K8sSecretStorage) (*KubernetesSecretStore, error) {
	if storage.Namespace == "" {
		// FIXME "default" as value by default ?
		storage.Namespace = getNamespace()
	}

	store := &KubernetesSecretStore{
		namespace:  storage.Namespace,
		secretName: storage.SecretName,
		storedData: make(map[string]*StoredData),
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("create in-cluster configuration: %w", err)
	}

	store.client, err = kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	return store, nil
}

// GetAccount returns ACME Account.
func (s *KubernetesSecretStore) GetAccount(resolverName string) (*Account, error) {
	secret, err := s.get(resolverName)
	if secret == nil || err != nil {
		return nil, err
	}

	return secret.Account, nil
}

// SaveAccount stores ACME Account.
func (s *KubernetesSecretStore) SaveAccount(resolverName string, account *Account) error {
	storedData, err := s.get(resolverName)
	if err != nil {
		return err
	}

	if storedData == nil {
		storedData = &StoredData{}
	}

	storedData.Account = account

	return s.save(resolverName, storedData)
}

// GetCertificates returns ACME Certificates list.
func (s *KubernetesSecretStore) GetCertificates(resolverName string) ([]*CertAndStore, error) {
	secret, err := s.get(resolverName)
	if secret == nil || err != nil {
		return nil, err
	}

	return secret.Certificates, nil
}

// SaveCertificates stores ACME Certificates list.
func (s *KubernetesSecretStore) SaveCertificates(resolverName string, certs []*CertAndStore) error {
	storedData, err := s.get(resolverName)
	if err != nil {
		return err
	}

	if storedData == nil {
		storedData = &StoredData{}
	}

	storedData.Certificates = certs

	return s.save(resolverName, storedData)
}

func (s *KubernetesSecretStore) save(resolverName string, storedData *StoredData) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.storedData[resolverName] = storedData

	dataAccount, err := json.Marshal(storedData)
	if err != nil {
		return err
	}

	patches := []struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value []byte `json:"value"`
	}{
		{
			Op:    "replace",
			Path:  "/data/" + resolverName,
			Value: dataAccount,
		},
	}

	payload, err := json.Marshal(patches)
	if err != nil {
		return err
	}

	_, err = s.client.CoreV1().Secrets(s.namespace).Patch(context.Background(), s.secretName, types.JSONPatchType, payload, metav1.PatchOptions{})
	if k8serrors.IsNotFound(err) {
		log.WithoutContext().Debugf("got error %+v when writing ACME KubernetesSecret, writing...", err)
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.secretName,
				Namespace: s.namespace,
			},
			Data: map[string][]byte{
				resolverName: dataAccount,
			},
		}
		_, err = s.client.CoreV1().Secrets(s.namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}

	return nil
}

func (s *KubernetesSecretStore) get(resolverName string) (*StoredData, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if data, exists := s.storedData[resolverName]; exists {
		return data, nil
	}

	secret, err := s.client.CoreV1().Secrets(s.namespace).Get(context.Background(), s.secretName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch secret %q: %w", s.secretName, err)
	}

	rawData, exists := secret.Data[resolverName]
	if !exists {
		return nil, nil
	}

	var data StoredData
	if err := json.Unmarshal(rawData, &data); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	// Delete all certificates with no value
	var certificates []*CertAndStore
	for _, certificate := range data.Certificates {
		if len(certificate.Certificate.Certificate) == 0 || len(certificate.Key) == 0 {
			log.WithoutContext().Debugf("Deleting empty certificate %v for %v", certificate, certificate.Domain.ToStrArray())
			continue
		}
		certificates = append(certificates, certificate)
	}
	if len(certificates) < len(data.Certificates) {
		data.Certificates = certificates
	}

	s.storedData[resolverName] = &data
	return &data, nil
}

// getNamespace returns the namespace in inCluster context
// see https://github.com/kubernetes/kubernetes/pull/63707
func getNamespace() string {
	// This way assumes you've set the POD_NAMESPACE environment variable using the downward API.
	// This check has to be done first for backwards compatibility with the way InClusterConfig was originally set up
	if ns, ok := os.LookupEnv("POD_NAMESPACE"); ok {
		return ns
	}

	// Fall back to the namespace associated with the service account token, if available
	if data, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			return ns
		}
	}

	return metav1.NamespaceDefault
}
