package cluster

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

const (
	serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"
	defaultNamespace  = "default"
)

type Metadata struct {
	Name            string            `json:"name,omitempty"`
	Namespace       string            `json:"namespace,omitempty"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type ConfigMap struct {
	APIVersion string            `json:"apiVersion,omitempty"`
	Kind       string            `json:"kind,omitempty"`
	Metadata   Metadata          `json:"metadata"`
	Data       map[string]string `json:"data,omitempty"`
}

type apiStatus struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type Client struct {
	baseURL   string
	namespace string
	token     string
	http      *http.Client
}

func NewInClusterClient(namespaceOverride string) (*Client, error) {
	host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST"))
	port := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_PORT"))
	if host == "" || port == "" {
		return nil, errors.New("Kubernetes service env vars are not available")
	}
	tokenBytes, err := os.ReadFile(path.Join(serviceAccountDir, "token"))
	if err != nil {
		return nil, err
	}
	namespace := strings.TrimSpace(namespaceOverride)
	if namespace == "" {
		if nsBytes, err := os.ReadFile(path.Join(serviceAccountDir, "namespace")); err == nil {
			namespace = strings.TrimSpace(string(nsBytes))
		}
	}
	if namespace == "" {
		namespace = defaultNamespace
	}

	roots := x509.NewCertPool()
	if caBytes, err := os.ReadFile(path.Join(serviceAccountDir, "ca.crt")); err == nil {
		roots.AppendCertsFromPEM(caBytes)
	}

	return &Client{
		baseURL:   fmt.Sprintf("https://%s:%s", host, port),
		namespace: namespace,
		token:     strings.TrimSpace(string(tokenBytes)),
		http: &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
			},
		},
	}, nil
}

func (c *Client) Namespace() string {
	return c.namespace
}

func (c *Client) GetConfigMap(ctx context.Context, name string) (*ConfigMap, bool, error) {
	var cm ConfigMap
	status, err := c.doJSON(ctx, http.MethodGet, c.configMapPath(name), nil, &cm)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status < 200 || status >= 300 {
		return nil, false, fmt.Errorf("get configmap %s returned HTTP %d", name, status)
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	return &cm, true, nil
}

func (c *Client) CreateConfigMap(ctx context.Context, cm *ConfigMap) error {
	cm.APIVersion = "v1"
	cm.Kind = "ConfigMap"
	cm.Metadata.Namespace = c.namespace
	status, err := c.doJSON(ctx, http.MethodPost, c.configMapsPath(), cm, nil)
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		return ErrConflict
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("create configmap %s returned HTTP %d", cm.Metadata.Name, status)
	}
	return nil
}

func (c *Client) UpdateConfigMap(ctx context.Context, cm *ConfigMap) error {
	cm.APIVersion = "v1"
	cm.Kind = "ConfigMap"
	cm.Metadata.Namespace = c.namespace
	status, err := c.doJSON(ctx, http.MethodPut, c.configMapPath(cm.Metadata.Name), cm, nil)
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		return ErrConflict
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("update configmap %s returned HTTP %d", cm.Metadata.Name, status)
	}
	return nil
}

var ErrConflict = errors.New("resource version conflict")

func (c *Client) UpdateConfigMapData(ctx context.Context, name string, labels map[string]string, mutate func(map[string]string) error) error {
	for attempt := 0; attempt < 8; attempt++ {
		cm, exists, err := c.GetConfigMap(ctx, name)
		if err != nil {
			return err
		}
		if !exists {
			data := map[string]string{}
			if err := mutate(data); err != nil {
				return err
			}
			err := c.CreateConfigMap(ctx, &ConfigMap{
				Metadata: Metadata{Name: name, Labels: labels},
				Data:     data,
			})
			if errors.Is(err, ErrConflict) {
				time.Sleep(backoff(attempt))
				continue
			}
			return err
		}

		data := copyStringMap(cm.Data)
		if err := mutate(data); err != nil {
			return err
		}
		cm.Data = data
		if cm.Metadata.Labels == nil {
			cm.Metadata.Labels = labels
		}
		err = c.UpdateConfigMap(ctx, cm)
		if errors.Is(err, ErrConflict) {
			time.Sleep(backoff(attempt))
			continue
		}
		return err
	}
	return ErrConflict
}

func (c *Client) LoadJSON(ctx context.Context, name, key string, target any) (bool, error) {
	cm, exists, err := c.GetConfigMap(ctx, name)
	if err != nil || !exists {
		return false, err
	}
	value := strings.TrimSpace(cm.Data[key])
	if value == "" {
		return false, nil
	}
	return true, json.Unmarshal([]byte(value), target)
}

func (c *Client) SaveJSON(ctx context.Context, name, key string, labels map[string]string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.UpdateConfigMapData(ctx, name, labels, func(data map[string]string) error {
		data[key] = string(payload)
		return nil
	})
}

func (c *Client) doJSON(ctx context.Context, method, urlPath string, body any, target any) (int, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+urlPath, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, nil
	}
	if target != nil && len(data) > 0 {
		if err := json.Unmarshal(data, target); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func (c *Client) configMapsPath() string {
	return fmt.Sprintf("/api/v1/namespaces/%s/configmaps", c.namespace)
}

func (c *Client) configMapPath(name string) string {
	return fmt.Sprintf("%s/%s", c.configMapsPath(), name)
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 5 {
		attempt = 5
	}
	return time.Duration(25*(1<<attempt)) * time.Millisecond
}
