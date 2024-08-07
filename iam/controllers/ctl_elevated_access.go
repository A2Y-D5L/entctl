package controllers

import (
  "context"
  "time"

  "path/to/elevatedaccessrequests/api/v1alpha1"
  iampv1alpha1 "path/to/iamuserpermissions/api/v1alpha1"
  "sigs.k8s.io/controller-runtime/pkg/client"
  "sigs.k8s.io/controller-runtime/pkg/controller"
  "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
  "sigs.k8s.io/controller-runtime/pkg/log"
  "sigs.k8s.io/controller-runtime/pkg/manager"
  "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ElevatedAccessRequestReconciler struct {
  client.Client
  Scheme *runtime.Scheme
}

func (r *ElevatedAccessRequestReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
  logger := log.FromContext(ctx)

  // Fetch the ElevatedAccessRequest instance
  var ear v1alpha1.ElevatedAccessRequest
  if err := r.Get(ctx, req.NamespacedName, &ear); err != nil {
    return reconcile.Result{}, client.IgnoreNotFound(err)
  }

  // Handle elevated access request
  if err := r.handleElevatedAccess(ctx, &ear); err != nil {
    return reconcile.Result{}, err
  }

  // Requeue to check for expiration
  return reconcile.Result{RequeueAfter: time.Minute * 5}, nil
}

func (r *ElevatedAccessRequestReconciler) handleElevatedAccess(ctx context.Context, ear *v1alpha1.ElevatedAccessRequest) error {
  // Calculate expiration time
  ttl, err := time.ParseDuration(ear.Spec.Ttl)
  if err != nil {
    return err
  }
  expirationTime := ear.CreationTimestamp.Add(ttl)

  if time.Now().After(expirationTime) {
    // Handle expiration
    if err := r.revokeElevatedAccess(ctx, ear); err != nil {
      return err
    }
    ear.Status.State = "Expired"
  } else {
    // Ensure elevated access is active
    if err := r.grantElevatedAccess(ctx, ear); err != nil {
      return err
    }
    ear.Status.State = "Active"
  }

  ear.Status.LastUpdated = metav1.Now()
  return r.Status().Update(ctx, ear)
}

func (r *ElevatedAccessRequestReconciler) grantElevatedAccess(ctx context.Context, ear *v1alpha1.ElevatedAccessRequest) error {
  iamp := &iampv1alpha1.IAMUserPermission{
    ObjectMeta: metav1.ObjectMeta{
      Name:      fmt.Sprintf("%s-elevated-%s", ear.Spec.ProductName, ear.Spec.UserEmail),
      Namespace: ear.Namespace,
    },
    Spec: iampv1alpha1.IAMUserPermissionSpec{
      UserEmail:         ear.Spec.UserEmail,
      ProductName:       ear.Spec.ProductName,
      Permissions:       ear.Spec.ElevatedPermissions,
      AssetNames:        ear.Spec.AssetNames,
      TemporaryPermissions: []iampv1alpha1.TemporaryPermission{
        {
          Permission: ear.Spec.ElevatedPermissions,
          Ttl:        ear.Spec.Ttl,
        },
      },
    },
  }

  _, err := controllerutil.CreateOrUpdate(ctx, r.Client, iamp, func() error {
    iamp.Spec.UserEmail = ear.Spec.UserEmail
    iamp.Spec.ProductName = ear.Spec.ProductName
    iamp.Spec.Permissions = ear.Spec.ElevatedPermissions
    iamp.Spec.AssetNames = ear.Spec.AssetNames
    return nil
  })

  return err
}

func (r *ElevatedAccessRequestReconciler) revokeElevatedAccess(ctx context.Context, ear *v1alpha1.ElevatedAccessRequest) error {
  iamp := &iampv1alpha1.IAMUserPermission{
    ObjectMeta: metav1.ObjectMeta{
      Name:      fmt.Sprintf("%s-elevated-%s", ear.Spec.ProductName, ear.Spec.UserEmail),
      Namespace: ear.Namespace,
    },
  }

  return r.Delete(ctx, iamp)
}

func (r *ElevatedAccessRequestReconciler) SetupWithManager(mgr manager.Manager) error {
  return controller.NewControllerManagedBy(mgr).
    For(&v1alpha1.ElevatedAccessRequest{}).
    Complete(r)
}