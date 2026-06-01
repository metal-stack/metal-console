package metalutil

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/metal-stack/api/go/client"
)

type (
	secret struct {
		namespace  string
		secretName string
		key        string
		clientset  kubernetes.Interface
	}
)

func NewPersistTokenFunc(namespace, secretName, key string, clientset kubernetes.Interface) (client.PersistTokenFn, error) {
	if namespace == "" || secretName == "" || key == "" {
		return nil, fmt.Errorf("namespace, secretName and key are required")
	}
	if clientset == nil {
		return nil, fmt.Errorf("clientset is required")
	}
	s := &secret{
		namespace:  namespace,
		secretName: secretName,
		key:        key,
		clientset:  clientset,
	}
	return s.persist, nil
}

func (s *secret) persist(token string) error {
	ctx := context.Background()
	sec, err := s.clientset.CoreV1().Secrets(s.namespace).Get(ctx, s.secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get secret:%w", err)
	}
	if sec.Data == nil {
		sec.Data = make(map[string][]byte)
	}
	sec.Data[s.key] = []byte(token)
	_, err = s.clientset.CoreV1().Secrets(s.namespace).Update(ctx, sec, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("unable to update secret:%w", err)
	}
	return nil
}
