package controllers

import (
  "context"
  "errors"
  "strings"
  "time"

  orgv1alpha1 "path/to/organizations/api/v1alpha1"
  "sigs.k8s.io/controller-runtime/pkg/client"
  "sigs.k8s.io/controller-runtime/pkg/controller"
  "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
  "sigs.k8s.io/controller-runtime/pkg/log"
  "sigs.k8s.io/controller-runtime/pkg/manager"
  "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type OrganizationReconciler struct {
  client.Client
  Scheme *runtime.Scheme
}

func (r *OrganizationReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
  logger := log.FromContext(ctx)

  // Fetch the Organization instance
  var organization orgv1alpha1.Organization
  if err := r.Get(ctx, req.NamespacedName, &organization); err != nil {
    return reconcile.Result{}, client.IgnoreNotFound(err)
  }

  // Validate references
  if err := r.validateReferences(ctx, &organization); err != nil {
    return reconcile.Result{}, err
  }

  // Add finalizers
  if err := r.addFinalizers(ctx, &organization); err != nil {
    return reconcile.Result{}, err
  }

  // Resolve approvers and members
  if err := r.resolveUserDetails(ctx, &organization); err != nil {
    return reconcile.Result{}, err
  }

  return reconcile.Result{}, nil
}

func (r *OrganizationReconciler) validateReferences(ctx context.Context, organization *orgv1alpha1.Organization) error {
  // Validate ownedBy reference
  if err := r.validateReference(ctx, organization.Spec.OwnedBy); err != nil {
    return err
  }

  // Validate memberOf references
  for _, ref := range organization.Spec.MemberOf {
    if err := r.validateReference(ctx, ref); err != nil {
      return err
    }
  }

  return nil
}

func (r *OrganizationReconciler) validateReference(ctx context.Context, ref orgv1alpha1.OrganizationReference) error {
  var org orgv1alpha1.Organization
  key := client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}
  if err := r.Get(ctx, key, &org); err != nil {
    return err
  }
  return nil
}

func (r *OrganizationReconciler) addFinalizers(ctx context.Context, organization *orgv1alpha1.Organization) error {
  if !controllerutil.ContainsFinalizer(organization, "finalizer.organization.cloud.company.com") {
    controllerutil.AddFinalizer(organization, "finalizer.organization.cloud.company.com")
    if err := r.Update(ctx, organization); err != nil {
      return err
    }
  }

  return nil
}

func (r *OrganizationReconciler) resolveUserDetails(ctx context.Context, organization *orgv1alpha1.Organization) error {
  // Example placeholder function to resolve AD group to user details
  resolveGroup := func(group string) ([]string, error) {
    return []string{"user1@example.com", "user2@example.com"}, nil
  }

  approvers, err := resolveGroup(organization.Spec.Approvers)
  if err != nil {
    return err
  }

  members, err := resolveGroup(organization.Spec.Members)
  if err != nil {
    return err
  }

  organization.Status.Approvers = approvers
  organization.Status.Members = members

  return r.Status().Update(ctx, organization)
}

func (r *OrganizationReconciler) SetupWithManager(mgr manager.Manager) error {
  return controller.NewControllerManagedBy(mgr).
    For(&orgv1alpha1.Organization{}).
    Complete(r)
}