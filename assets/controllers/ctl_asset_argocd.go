package controllers

import (
	"context"
	"fmt"

	assetsv1alpha1 "path/to/assets/api/v1alpha1"

	"github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AssetReconciler_ArgoCD struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *AssetReconciler_ArgoCD) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Asset instance
	var asset assetsv1alpha1.Asset
	if err := r.Get(ctx, req.NamespacedName, &asset); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Reconcile Argo CD application for the asset
	if err := r.reconcileArgoCDApplication(ctx, &asset); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *AssetReconciler_ArgoCD) reconcileArgoCDApplication(ctx context.Context, asset *assetsv1alpha1.Asset) error {
	// Create the Argo CD Application object
	app := &v1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-claims", asset.Name),
			Namespace: "argocd",
		},
		Spec: v1alpha1.ApplicationSpec{
			Project: "default",
			Source: v1alpha1.ApplicationSource{
				RepoURL:        asset.Spec.Claims.RepositoryURL,
				TargetRevision: asset.Spec.Claims.GitCommit,
				Path:           "claims",
			},
			Destination: v1alpha1.ApplicationDestination{
				Server:    "https://kubernetes.default.svc",
				Namespace: asset.Namespace,
			},
			SyncPolicy: &v1alpha1.SyncPolicy{
				Automated: &v1alpha1.SyncPolicyAutomated{
					Prune:    true,
					SelfHeal: true,
				},
				SyncOptions: []string{"CreateNamespace=true"},
			},
		},
	}

	// Create or update the Argo CD Application resource
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, app, func() error {
		app.Spec.Source.RepoURL = asset.Spec.Claims.RepositoryURL
		app.Spec.Source.TargetRevision = asset.Spec.Claims.GitCommit
		app.Spec.Source.Path = "claims"
		app.Spec.Destination.Namespace = asset.Namespace
		return controllerutil.SetControllerReference(asset, app, r.Scheme)
	})

	return err
}

func (r *AssetReconciler_ArgoCD) SetupWithManager(mgr manager.Manager) error {
	return controller.NewControllerManagedBy(mgr).
		For(&assetsv1alpha1.Asset{}).
		Complete(r)
}
