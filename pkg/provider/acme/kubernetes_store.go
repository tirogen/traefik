package acme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/traefik/traefik/v2/pkg/log"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// FieldManager is the name of this process writing to k8s.
const FieldManager = "traefik"

// LabelResolver is the key of the Kubernetes label where we store the secret's
// resolver name.
const LabelResolver = "traefik.ingress.kubernetes.io/resolver"

// LabelACMEStorage is the key of the Kubernetes label that marks a sercet as
// stored.
const LabelACMEStorage = "traefik.ingress.kubernetes.io/acme-storage"

// KubernetesSecretStore stores ACME account and certificates Kubernetes secrets.
// Each resolver gets it's own secrets and each domain is stored as a separate
// value in the secret.
// All secrets managed by this store well get the label
// `traefik.ingress.kubernetes.io/acme-storage=true`.
type KubernetesSecretStore struct {
	ctx context.Context

	namespace  string
	secretName string
	client     kubernetes.Interface

	lock       *sync.Mutex
	storedData map[string]*StoredData
}

// NewKubernetesSecretStore will initiate a new KubernetesSecretStore, create a Kubernetes
// clientset with the default 'in-cluster' config.
func NewKubernetesSecretStore(storage *K8sSecretStorage) (*KubernetesSecretStore, error) {
	if storage.Namespace == "" {
		storage.Namespace = "default"
	}

	store := &KubernetesSecretStore{
		ctx:        log.With(context.Background(), log.Str(log.ProviderName, "k8s-secret-acme"), log.Str("SecretName", storage.SecretName)),
		namespace:  storage.Namespace,
		secretName: storage.SecretName,
		lock:       &sync.Mutex{},
		storedData: make(map[string]*StoredData),
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster configuration: %w", err)
	}

	if storage.Endpoint != "" {
		config.Host = storage.Endpoint
	}

	store.client, err = kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return store, nil
}

func (s *KubernetesSecretStore) save(resolverName string, storedData *StoredData) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	logger := log.FromContext(s.ctx)

	dataAccount, err := json.MarshalIndent(storedData.Account, "", "  ")
	if err != nil {
		return err
	}

	patches := []patch{
		{
			Op:    "replace",
			Path:  "/data/account",
			Value: dataAccount,
		},
	}

	for _, cert := range storedData.Certificates {
		if cert.Domain.Main == "" {
			logger.Warn("not saving a certificate without a main domain name")
			continue
		}

		data, err := json.Marshal(cert)
		if err != nil {
			return fmt.Errorf("failed to marshale account: %w", err)
		}

		patches = append(patches, patch{
			Op:    "replace",
			Path:  "/data/" + cert.Domain.Main,
			Value: data,
		})
	}

	payload, err := json.MarshalIndent(patches, "", "  ")
	if err != nil {
		return err
	}

	status := &k8serrors.StatusError{}
	_, err = s.client.CoreV1().Secrets(s.namespace).Patch(s.ctx, s.secretName, types.JSONPatchType, payload, metav1.PatchOptions{FieldManager: FieldManager})
	if err != nil && errors.As(err, &status) && status.Status().Code == 404 {
		logger.Debugf("got error %+v when writing ACME Secret, writing...", err)
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: s.secretName,
				Labels: map[string]string{
					LabelACMEStorage: "true",
					LabelResolver:    resolverName,
				},
			},
			Data: map[string][]byte{
				"account": payload,
			},
		}
		_, err = s.client.CoreV1().Secrets(s.namespace).Create(s.ctx, secret, metav1.CreateOptions{FieldManager: FieldManager})
	}
	if err != nil {
		return err
	}

	s.storedData[resolverName] = storedData

	return nil
}

func (s *KubernetesSecretStore) get(resolverName string) (*StoredData, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.storedData == nil {
		s.storedData = make(map[string]*StoredData)
		secret, err := s.client.CoreV1().Secrets(s.namespace).Get(s.ctx, s.secretName, metav1.GetOptions{})
		status := &k8serrors.StatusError{}
		if err != nil {
			if errors.As(err, &status) && status.Status().Code == 404 {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to fetch secret %q: %w", s.secretName, err)
		}

		if err := json.Unmarshal(secret.Data["account"], s.storedData[resolverName].Account); err != nil {
			return nil, err
		}

		for domain, data := range secret.Data {
			if domain == "account" {
				continue
			}

			var certAndStore CertAndStore
			if err = json.Unmarshal(data, &certAndStore); err != nil {
				return nil, err
			}

			if domain != certAndStore.Domain.Main {
				log.FromContext(s.ctx).Warnf("mismatch in cert domain and secret keyname: %q != %q", domain, certAndStore.Domain.Main)
			}

			s.storedData[resolverName].Certificates = append(s.storedData[resolverName].Certificates, &certAndStore)
		}
	}

	if s.storedData[resolverName] == nil {
		s.storedData[resolverName] = &StoredData{}
	}

	return s.storedData[resolverName], nil
}

// GetAccount returns the account information for the given resolverName, this
// either from storedData (which is maintained by the watcher and Save* operations)
// or it will fetch the resource fresh.
func (s *KubernetesSecretStore) GetAccount(resolverName string) (*Account, error) {
	secret, err := s.get(resolverName)
	if secret == nil || err != nil {
		return nil, err
	}

	return secret.Account, nil
}

// SaveAccount will patch the kubernetes secret resource for the given
// resolverName with the given account data. When the secret did not exist it is
// created with the correct labels set.
func (s *KubernetesSecretStore) SaveAccount(resolverName string, account *Account) error {
	storedData, err := s.get(resolverName)
	if err != nil {
		return err
	}

	storedData.Account = account

	return s.save(resolverName, storedData)
}

// GetCertificates returns all certificates for the given resolverName, this
// either from storedData (which is maintained by the watcher and Save* operations)
// or it will fetch the resource fresh.
func (s *KubernetesSecretStore) GetCertificates(resolverName string) ([]*CertAndStore, error) {
	secret, err := s.get(resolverName)
	if secret == nil || err != nil {
		return nil, err
	}

	return secret.Certificates, nil
}

// SaveCertificates will patch the kubernetes secret resource for the given
// resolverName with the given certificates. When the secret did not exist it is
// created with the correct labels set.
func (s *KubernetesSecretStore) SaveCertificates(resolverName string, certs []*CertAndStore) error {
	storedData, err := s.get(resolverName)
	if err != nil {
		return err
	}

	storedData.Certificates = certs

	return s.save(resolverName, storedData)
}

type patch struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value []byte `json:"value"`
}
