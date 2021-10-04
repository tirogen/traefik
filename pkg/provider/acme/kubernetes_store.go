package acme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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

// KubernetesStore stores ACME account and certificates Kubernetes secrets.
// Each resolver gets it's own secrets and each domain is stored as a separate
// value in the secret.
// All secrets managed by this store well get the label
// `traefik.ingress.kubernetes.io/acme-storage=true`.
type KubernetesStore struct {
	ctx context.Context

	namespace string
	client    kubernetes.Interface

	lock       *sync.Mutex
	storedData map[string]*StoredData
}

// KubernetesStoreFromURI will create a new KubernetesStore instance from the
// given URI with this format: `kubernetes://:endpoint:/:namespace:`. The endpoint
// (or host:port part) of the uri is optional. Example: `kubernetes:///default`
func KubernetesStoreFromURI(uri string) (*KubernetesStore, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %q: %w", uri, err)
	}
	namespace := u.Path[1:]
	endpoint := ""
	if u.Host != "" {
		endpoint = u.Host
	}

	return NewKubernetesStore(namespace, endpoint)
}

// NewKubernetesStore will initiate a new KubernetesStore, create a Kubernetes
// clientset and start a resource watcher for stored sercrets.
// It will create a clientset with the default 'in-cluster' config.
func NewKubernetesStore(namespace, endpoint string) (*KubernetesStore, error) {
	if namespace == "" {
		namespace = "default"
	}

	store := &KubernetesStore{
		ctx:        log.With(context.Background(), log.Str(log.ProviderName, "k8s-secret-acme")),
		namespace:  namespace,
		lock:       &sync.Mutex{},
		storedData: make(map[string]*StoredData),
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster configuration: %w", err)
	}

	if endpoint != "" {
		config.Host = endpoint
	}

	store.client, err = kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return store, nil
}

func (s *KubernetesStore) save(resolverName string, storedData *StoredData) error {
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
	_, err = s.client.CoreV1().Secrets(s.namespace).Patch(s.ctx, secretName(resolverName), types.JSONPatchType, payload, metav1.PatchOptions{FieldManager: FieldManager})
	if err != nil && errors.As(err, &status) && status.Status().Code == 404 {
		logger.Debugf("got error %+v when writing ACME Secret, writing...", err)
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: secretName(resolverName),
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

func (s *KubernetesStore) get(resolverName string) (*StoredData, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.storedData == nil {
		s.storedData = make(map[string]*StoredData)
		secret, err := s.client.CoreV1().Secrets(s.namespace).Get(s.ctx, secretName(resolverName), metav1.GetOptions{})
		status := &k8serrors.StatusError{}
		if err != nil {
			if errors.As(err, &status) && status.Status().Code == 404 {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to fetch secret %q: %w", secretName(resolverName), err)
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
func (s *KubernetesStore) GetAccount(resolverName string) (*Account, error) {
	secret, err := s.get(resolverName)
	if secret == nil || err != nil {
		return nil, err
	}

	return secret.Account, nil
}

// SaveAccount will patch the kubernetes secret resource for the given
// resolverName with the given account data. When the secret did not exist it is
// created with the correct labels set.
func (s *KubernetesStore) SaveAccount(resolverName string, account *Account) error {
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
func (s *KubernetesStore) GetCertificates(resolverName string) ([]*CertAndStore, error) {
	secret, err := s.get(resolverName)
	if secret == nil || err != nil {
		return nil, err
	}

	return secret.Certificates, nil
}

// SaveCertificates will patch the kubernetes secret resource for the given
// resolverName with the given certificates. When the secret did not exist it is
// created with the correct labels set.
func (s *KubernetesStore) SaveCertificates(resolverName string, certs []*CertAndStore) error {
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

func secretName(resolverName string) string {
	return "traefik-acme-" + resolverName + "-storage"
}
