package acme

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v2/pkg/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesStoreAccounts(t *testing.T) {
	resolver := "resolver01"
	testCases := []struct {
		desc        string
		account     Account
		accountEdit *Account
		storedData  StoredData
	}{
		{desc: "Empty"},
		{
			desc: "With account",
			account: Account{
				Email:      "john@example.org",
				KeyType:    certcrypto.RSA2048,
				PrivateKey: []byte("0123456789"),
			},
		},
		{
			desc: "With other account in secret",
			storedData: StoredData{
				Account: &Account{
					Email: "john@example.org",
				},
			},
		},
		{
			desc: "With account edition",
			account: Account{
				Email:      "john@example.org",
				KeyType:    certcrypto.RSA2048,
				PrivateKey: []byte("0123456789"),
			},
			accountEdit: &Account{
				Email: "not-john@example.org",
			},
		},
	}

	for _, test := range testCases {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			store := &KubernetesSecretStore{
				client:     fake.NewSimpleClientset(),
				lock:       sync.Mutex{},
				secretName: "test-secret",
				storedData: make(map[string]*StoredData),
			}

			setupKubernetesSecret(t, store, resolver, test.storedData)

			got, err := store.GetAccount(resolver)
			require.NoError(t, err)
			if test.storedData.Account != nil {
				if !reflect.DeepEqual(test.storedData.Account, got) {
					t.Errorf("expected account %v, got %v instead",
						test.storedData.Account, got)
				}
			} else {
				assert.Nil(t, got)
			}

			err = store.SaveAccount(resolver, &test.account)
			require.NoError(t, err)

			got, err = store.GetAccount(resolver)
			require.NoError(t, err)

			if !reflect.DeepEqual(test.account, *got) {
				t.Errorf("expected account %v, got %v instead",
					test.account, *got)
			}

			if test.accountEdit != nil {
				err := store.SaveAccount(resolver, test.accountEdit)
				require.NoError(t, err)

				got, err := store.GetAccount(resolver)
				require.NoError(t, err)

				if !reflect.DeepEqual(test.accountEdit, got) {
					t.Errorf("after edition, expected account %v, got %v instead",
						test.accountEdit, got)
				}
			}
		})
	}
}

func TestKubernetesStoreCertificates(t *testing.T) {
	resolver := "resolver01"
	testCases := []struct {
		desc       string
		certs      []*CertAndStore
		storedData StoredData
	}{
		{desc: "Empty"},
		{
			desc: "With certificates in secret to remove",
			storedData: StoredData{
				Certificates: []*CertAndStore{
					{
						Certificate: Certificate{
							Certificate: []byte("9876543210"),
							Domain:      types.Domain{Main: "1.example.org"},
							Key:         []byte("9876543210"),
						},
					},
					{
						Certificate: Certificate{
							Certificate: []byte("9876543210"),
							Domain:      types.Domain{Main: "2.example.org"},
						},
					},
				},
			},
		},
		{
			desc: "With domain",
			certs: []*CertAndStore{
				{
					Certificate: Certificate{
						Certificate: []byte("0123456789"),
						Domain:      types.Domain{Main: "example.org"},
						Key:         []byte("0123456789"),
					},
					Store: "store01",
				},
			},
		},
		{
			desc: "With domain and sub",
			certs: []*CertAndStore{
				{
					Certificate: Certificate{
						Certificate: []byte("0123456789"),
						Domain:      types.Domain{Main: "example.org"},
						Key:         []byte("0123456789"),
					},
					Store: "store01",
				},
				{
					Certificate: Certificate{
						Certificate: []byte("9876543210"),
						Domain:      types.Domain{Main: "sub.example.org"},
						Key:         []byte("9876543210"),
					},
					Store: "store02",
				},
			},
		},
	}

	for _, test := range testCases {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			store := &KubernetesSecretStore{
				client:     fake.NewSimpleClientset(),
				lock:       sync.Mutex{},
				secretName: "test-secret",
				storedData: make(map[string]*StoredData),
			}

			got, err := store.GetCertificates(resolver)
			require.NoError(t, err)
			require.Nil(t, got)

			err = store.SaveCertificates(resolver, test.certs)
			require.NoError(t, err)

			got, err = store.GetCertificates(resolver)
			require.NoError(t, err)

			if !reflect.DeepEqual(test.certs, got) {
				t.Errorf("expected certs %v, got %v instead",
					test.certs, got)
			}
		})
	}
}

func setupKubernetesSecret(t *testing.T, store *KubernetesSecretStore, resolver string, data StoredData) {
	t.Helper()

	dataAccount, err := json.Marshal(data)
	require.NoError(t, err)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: store.secretName,
		},
		Data: map[string][]byte{
			resolver: dataAccount,
		},
	}
	_, err = store.client.CoreV1().Secrets(store.namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	require.NoError(t, err)
}
