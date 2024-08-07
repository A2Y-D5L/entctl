package controllers

import (
	"context"
	"fmt"
	"hash/fnv"

	iampv1alpha1 "path/to/iamuserpermissions/api/v1alpha1"
	orgv1alpha1 "path/to/organizations/api/v1alpha1"
	prodv1alpha1 "path/to/products/api/v1alpha1"

	"github.com/crossplane-contrib/provider-gcp/apis/v1beta1"
	crossplanev1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ProductReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *ProductReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Product instance
	var product prodv1alpha1.Product
	if err := r.Get(ctx, req.NamespacedName, &product); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Validate organization exists
	if err := r.validateOrganization(ctx, &product); err != nil {
		return reconcile.Result{}, err
	}

	// Create GCP projects
	if err := r.createGCPProjects(ctx, &product); err != nil {
		return reconcile.Result{}, err
	}

	// Resolve user details for members and approvers, and create IAM permissions
	if err := r.resolveUserDetailsAndCreatePermissions(ctx, &product); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ProductReconciler) validateOrganization(ctx context.Context, product *prodv1alpha1.Product) error {
	var organization orgv1alpha1.Organization
	key := client.ObjectKey{Name: product.Spec.Organization.Name, Namespace: product.Namespace}
	if err := r.Get(ctx, key, &organization); err != nil {
		return err
	}
	return nil
}

func (r *ProductReconciler) createGCPProjects(ctx context.Context, product *prodv1alpha1.Product) error {
	shortHash := func(s string) string {
		h := fnv.New32a()
		h.Write([]byte(s))
		return fmt.Sprintf("%x", h.Sum32())
	}

	// Generate project names and IDs
	nonProdProjectID := fmt.Sprintf("%s-nonprod-%s", product.Spec.Name, shortHash(product.Spec.Name+"-nonprod"))
	prodProjectID := fmt.Sprintf("%s-prod-%s", product.Spec.Name, shortHash(product.Spec.Name+"-prod"))

	// Create non-prod project
	nonProdProject := &v1beta1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonProdProjectID,
			Namespace: product.Namespace,
		},
		Spec: v1beta1.ProjectSpec{
			ForProvider: v1beta1.ProjectParameters{
				Name: nonProdProjectID,
			},
			ProviderConfigReference: &crossplanev1.Reference{
				Name: "gcp-provider",
			},
		},
	}

	if err := r.Create(ctx, nonProdProject); err != nil {
		return err
	}

	// Create prod project
	prodProject := &v1beta1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prodProjectID,
			Namespace: product.Namespace,
		},
		Spec: v1beta1.ProjectSpec{
			ForProvider: v1beta1.ProjectParameters{
				Name: prodProjectID,
			},
			ProviderConfigReference: &crossplanev1.Reference{
				Name: "gcp-provider",
			},
		},
	}

	if err := r.Create(ctx, prodProject); err != nil {
		return err
	}

	return nil
}

func (r *ProductReconciler) resolveUserDetailsAndCreatePermissions(ctx context.Context, product *prodv1alpha1.Product) error {
	// Example placeholder function to resolve AD group to user details
	resolveGroup := func(group string) ([]string, error) {
		return []string{"user1@example.com", "user2@example.com"}, nil
	}

	// Fetch the organization
	var organization orgv1alpha1.Organization
	key := client.ObjectKey{Name: product.Spec.Organization.Name, Namespace: product.Namespace}
	if err := r.Get(ctx, key, &organization); err != nil {

		return err
	}

	// Resolve members and approvers
	members, err := resolveGroup(organization.Spec.Members)
	if err != nil {
		return err
	}

	approvers, err := resolveGroup(organization.Spec.Approvers)
	if err != nil {
		return err
	}

	// Create IAM permissions in the non-prod project for resolved members and approvers
	for _, userEmail := range append(members, approvers...) {
		if err := r.createIAMPermissions(ctx, product.Namespace, product.Spec.Name, userEmail, nonProdProjectID); err != nil {
			return err
		}
	}

	return nil
}

func (r *ProductReconciler) createIAMPermissions(ctx context.Context, namespace, productName, userEmail, projectID string) error {
	iamp := &iampv1alpha1.IAMUserPermission{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-iamp", productName, userEmail),
			Namespace: namespace,
		},
		Spec: iampv1alpha1.IAMUserPermissionSpec{
			UserEmail:   userEmail,
			ProductName: productName,
			Permissions: []string{"roles/viewer"}, // Default read-only access
			AssetNames:  []string{projectID},
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, iamp, func() error {
		iamp.Spec.UserEmail = userEmail
		iamp.Spec.ProductName = productName
		iamp.Spec.Permissions = []string{"roles/viewer"}
		iamp.Spec.AssetNames = []string{projectID}
		return nil
	})

	return err
}

func (r *ProductReconciler) SetupWithManager(mgr manager.Manager) error {
	return controller.NewControllerManagedBy(mgr).
		For(&prodv1alpha1.Product{}).
		Complete(r)
}
