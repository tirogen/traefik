package acme

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v2/pkg/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesStoreAccounts(t *testing.T) {
	resolver := "resolver01"
	testCases := []struct {
		desc        string
		account     Account
		accountEdit *Account
	}{
		{desc: "Empty"},
		{
			desc: "With account",
			account: Account{
				Email:      "john@example.org",
				PrivateKey: []byte("0123456789"),
				KeyType:    certcrypto.RSA2048,
			},
		},
		{
			desc: "With account edition",
			account: Account{
				Email:      "john@example.org",
				PrivateKey: []byte("0123456789"),
				KeyType:    certcrypto.RSA2048,
			},
			accountEdit: &Account{
				Email: "not-john@example.org",
			},
		},
	}

	for _, test := range testCases {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			store := &KubernetesStore{
				ctx:        ctx,
				lock:       &sync.Mutex{},
				storedData: make(map[string]v1.Secret),
				client:     fake.NewSimpleClientset(),
			}

			got, err := store.GetAccount(resolver)
			require.NoError(t, err)
			assert.Nil(t, got)

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
		desc  string
		certs []*CertAndStore
	}{
		{desc: "Empty"},
		{
			desc: "With domain",
			certs: []*CertAndStore{
				{
					Certificate: Certificate{
						Domain:      types.Domain{Main: "example.org"},
						Certificate: []byte("0123456789"),
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
						Domain:      types.Domain{Main: "example.org"},
						Certificate: []byte("0123456789"),
						Key:         []byte("0123456789"),
					},
					Store: "store01",
				},
				{
					Certificate: Certificate{
						Domain:      types.Domain{Main: "sub.example.org"},
						Certificate: []byte("9876543210"),
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
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			store := &KubernetesStore{
				ctx:        ctx,
				lock:       &sync.Mutex{},
				storedData: make(map[string]v1.Secret),
				client:     fake.NewSimpleClientset(),
			}

			err := store.SaveCertificates(resolver, test.certs)
			require.NoError(t, err)

			got, err := store.GetCertificates(resolver)
			require.NoError(t, err)

			if !reflect.DeepEqual(test.certs, got) {
				t.Errorf("expected certs %v, got %v instead",
					test.certs, got)
			}
		})
	}
}
