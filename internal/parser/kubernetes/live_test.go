package kubernetes

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFetchLive_KubectlNotFound(t *testing.T) {
	originalLookPath := kubectlLookPath
	kubectlLookPath = func(string) (string, error) {
		return "", errors.New("not found")
	}
	t.Cleanup(func() {
		kubectlLookPath = originalLookPath
	})

	_, err := FetchLive(context.Background(), "", "", []string{"default"})
	if err == nil {
		t.Fatal("expected error when kubectl is not found")
	}
}

func TestFetchLive_ListNamespacesError(t *testing.T) {
	originalLookPath := kubectlLookPath
	originalListNamespaces := listNamespacesFn
	kubectlLookPath = func(string) (string, error) {
		return "/usr/bin/kubectl", nil
	}
	listNamespacesFn = func(context.Context, string, string) ([]string, error) {
		return nil, context.DeadlineExceeded
	}
	t.Cleanup(func() {
		kubectlLookPath = originalLookPath
		listNamespacesFn = originalListNamespaces
	})

	_, err := FetchLive(context.Background(), "", "", nil)
	if err == nil {
		t.Fatal("expected error when listing namespaces fails")
	}
}

func TestFetchLive_CollectsWarningsAndContinues(t *testing.T) {
	originalLookPath := kubectlLookPath
	originalGet := kubectlGetFn
	kubectlLookPath = func(string) (string, error) {
		return "/usr/bin/kubectl", nil
	}
	kubectlGetFn = func(_ context.Context, _, _, namespace, resourceTypes string) ([]byte, error) {
		if resourceTypes == "certificates.cert-manager.io" {
			return nil, errors.New("no cert manager")
		}
		if namespace == "broken" {
			return nil, errors.New("cluster unreachable")
		}
		return []byte(`apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api
    namespace: default
`), nil
	}
	t.Cleanup(func() {
		kubectlLookPath = originalLookPath
		kubectlGetFn = originalGet
	})

	r, err := FetchLive(context.Background(), "", "", []string{"broken", "default"})
	if err != nil {
		t.Fatalf("FetchLive returned unexpected error: %v", err)
	}
	if len(r.Nodes) == 0 {
		t.Fatal("expected nodes from healthy namespace")
	}
	if len(r.Warnings) == 0 {
		t.Fatal("expected warning from broken namespace")
	}
}

func TestFetchLive_AppliesDefaultTimeoutWhenMissingDeadline(t *testing.T) {
	originalLookPath := kubectlLookPath
	originalListNamespaces := listNamespacesFn
	kubectlLookPath = func(string) (string, error) {
		return "/usr/bin/kubectl", nil
	}
	listNamespacesFn = func(ctx context.Context, _, _ string) ([]string, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline on context passed to listNamespaces")
		}
		return nil, context.DeadlineExceeded
	}
	t.Cleanup(func() {
		kubectlLookPath = originalLookPath
		listNamespacesFn = originalListNamespaces
	})

	_, _ = FetchLive(context.Background(), "", "", nil)
}

func TestBuildKubectlArgs(t *testing.T) {
	tests := []struct {
		name       string
		kubeconfig string
		kubeCtx    string
		want       int // expected arg count
	}{
		{"empty", "", "", 0},
		{"kubeconfig only", "/path/to/config", "", 2},
		{"context only", "", "my-context", 2},
		{"both", "/path/to/config", "my-context", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildKubectlArgs(tt.kubeconfig, tt.kubeCtx)
			if len(args) != tt.want {
				t.Errorf("buildKubectlArgs(%q, %q) = %d args, want %d", tt.kubeconfig, tt.kubeCtx, len(args), tt.want)
			}
		})
	}
}

func TestParseManifests_KubectlListOutput(t *testing.T) {
	// Simulates kubectl get deployments -o yaml which returns a List wrapper
	data := []byte(`apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: web-app
    namespace: staging
  spec:
    replicas: 2
    selector:
      matchLabels:
        app: web
    template:
      metadata:
        labels:
          app: web
      spec:
        containers:
        - name: web
          image: nginx:1.25
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api-server
    namespace: staging
  spec:
    replicas: 3
    selector:
      matchLabels:
        app: api
    template:
      metadata:
        labels:
          app: api
      spec:
        containers:
        - name: api
          image: mycompany/api:v1.0
`)

	result, err := parseManifests(data, "live:staging", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	nodeIDs := make(map[string]bool)
	for _, n := range result.Nodes {
		nodeIDs[n.ID] = true
	}

	if !nodeIDs["k8s:pod:staging/web-app"] {
		t.Error("missing k8s:pod:staging/web-app from List output")
	}
	if !nodeIDs["k8s:pod:staging/api-server"] {
		t.Error("missing k8s:pod:staging/api-server from List output")
	}
	if len(result.Nodes) != 2 {
		t.Errorf("nodes = %d, want 2", len(result.Nodes))
	}
}

func TestParseManifests_EmptyList(t *testing.T) {
	data := []byte(`apiVersion: v1
kind: List
items: []
`)

	result, err := parseManifests(data, "live:empty", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 0 {
		t.Errorf("nodes = %d, want 0 for empty list", len(result.Nodes))
	}
}
