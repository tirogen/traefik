package acme

import (
	"context"
	"encoding/json"
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
	namespace  string
	secretName string
	client     kubernetes.Interface

	lock       sync.Mutex
	storedData map[string]*StoredData
}

// NewKubernetesSecretStore creates a new KubernetesSecretStore instance.
func NewKubernetesSecretStore(storage *K8sSecretStorage) (*KubernetesSecretStore, error) {
	// FIXME change namespace by default
	store := &KubernetesSecretStore{
		namespace:  storage.Namespace,
		secretName: storage.SecretName,
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

func (s *KubernetesSecretStore) save(resolverName string, storedData *StoredData) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	logger := log.WithoutContext()

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

	_, err = s.client.CoreV1().Secrets(s.namespace).Patch(context.Background(), s.secretName, types.JSONPatchType, payload, metav1.PatchOptions{FieldManager: FieldManager})
	if k8serrors.IsNotFound(err) {
		logger.Debugf("got error %+v when writing ACME KubernetesSecret, writing...", err)
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
		_, err = s.client.CoreV1().Secrets(s.namespace).Create(context.Background(), secret, metav1.CreateOptions{FieldManager: FieldManager})
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

		secret, err := s.client.CoreV1().Secrets(s.namespace).Get(context.Background(), s.secretName, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("fetch secret %q: %w", s.secretName, err)
		}

		if err := json.Unmarshal(secret.Data["data"], &s.storedData); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}

		// Delete all certificates with no value
		var certificates []*CertAndStore
		for _, storedData := range s.storedData {
			for _, certificate := range storedData.Certificates {
				if len(certificate.Certificate.Certificate) == 0 || len(certificate.Key) == 0 {
					log.WithoutContext().Debugf("Deleting empty certificate %v for %v", certificate, certificate.Domain.ToStrArray())
					continue
				}
				certificates = append(certificates, certificate)
			}
			if len(certificates) < len(storedData.Certificates) {
				storedData.Certificates = certificates
			}
		}
	}

	if s.storedData[resolverName] == nil {
		s.storedData[resolverName] = &StoredData{}
	}
	return s.storedData[resolverName], nil
}

type patch struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value []byte `json:"value"`
}
