package controllers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	assetsv1alpha1 "path/to/assets/api/v1alpha1"

	"github.com/crossplane-contrib/provider-gcp/apis/v1beta1"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AssetReconciler_Custom struct {
	client.Client
	Scheme *runtime.Scheme
}
 
func (r *AssetReconciler_Custom) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Asset instance
	var asset assetsv1alpha1.Asset
	if err := r.Get(ctx, req.NamespacedName, &asset); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Reconcile asset claims
	if err := r.reconcileClaims(ctx, &asset); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *AssetReconciler_Custom) reconcileClaims(ctx context.Context, asset *assetsv1alpha1.Asset) error {
	claims, err := r.fetchClaimsFromRepo(asset.Spec.Claims.RepositoryURL, asset.Spec.Claims.GitBranch, asset.Spec.Claims.GitCommit)
	if err != nil {
		return err
	}

	for _, claim := range claims {
		if err := r.provisionClaim(ctx, asset, claim); err != nil {
			return err
		}
	}

	return nil
}

func (r *AssetReconciler_Custom) fetchClaimsFromRepo(repoURL, branch, commit string) ([]map[string]interface{}, error) {
	// Construct the URL to the raw content of the repository
	rawURL := fmt.Sprintf("%s/raw/%s/%s/", repoURL, commit, branch)
	resp, err := http.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch repository content: %s", resp.Status)
	}

	// Read the response body
	body, err := os.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Find and parse claim files
	var claims []map[string]interface{}
	files := filepath.Glob("*.claim.yaml")
	for _, file := range files {
		var claim map[string]interface{}
		if err := yaml.Unmarshal(body, &claim); err != nil {
			return nil, err
		}
		claims = append(claims, claim)
	}

	return claims, nil
}

func (r *AssetReconciler_Custom) provisionClaim(ctx context.Context, asset *assetsv1alpha1.Asset, claim map[string]interface{}) error {
	// Convert claim to the appropriate Crossplane resource type
	claimResource := &v1beta1.Project{}
	if err := mapstructure.Decode(claim, claimResource); err != nil {
		return err
	}

	// Set metadata and owner references
	claimResource.SetNamespace(asset.Namespace)
	claimResource.SetName(fmt.Sprintf("%s-%s-claim", asset.Name, claimResource.GetName()))
	if err := controllerutil.SetControllerReference(asset, claimResource, r.Scheme); err != nil {
		return err
	}

	// Create or update the claim resource in the cluster
	if err := r.Create(ctx, claimResource); err != nil {
		return err
	}

	return nil
}

func (r *AssetReconciler_Custom) SetupWithManager(mgr manager.Manager) error {
	return controller.NewControllerManagedBy(mgr).
		For(&assetsv1alpha1.Asset{}).
		Complete(r)
}
