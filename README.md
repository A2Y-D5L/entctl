# A2Y-D5L / entctl

`entctl` is a proof-of-concept design for an enterprise cloud control plane. It leverages Kubernetes custom resources and controllers to manage organizational structures, products, assets, and IAM permissions, integrating with Crossplane for managing GCP resources and Argo CD for GitOps-based synchronization of asset claims stored in Bitbucket. Here’s a detailed summary of the components and their relationships:

#### Custom Resource Definitions (CRDs)

1. **Organizations CRD**
   - **Purpose**: Represents organizational units within the company, detailing ownership, hierarchical structure, and access control groups.
   - **Relationships**: References other organizations (`ownedBy`, `memberOf`) and defines user groups for approvers and members.

2. **Products CRD**
   - **Purpose**: Represents products within an organization, each associated with GCP projects.
   - **Relationships**: References an `Organization` and encompasses multiple `Assets`.

3. **Assets CRD**
   - **Purpose**: Represents assets within a product, detailing resource deployment information.
   - **Status**: Tracks the state and messages related to the asset.

4. **IAMUserPermissions CRD**
   - **Purpose**: Maps IAM permissions between users, products, and assets.
   - **Status**: Tracks the state and messages related to the permissions.

5. **ElevatedAccessRequest CRD**
   - **Purpose**: Represents requests for temporary elevated access.
   - **Status**: Tracks the state, messages, and expiration details of the request.

#### Controllers

1. **Organizations Controller**
   - **Responsibilities**: Validates organizational references, adds finalizers, resolves user details for approvers and members, and updates status accordingly.
   - **Integration**: Ensures proper management of organizational structures and user groups.

2. **Products Controller**
   - **Responsibilities**: Validates referenced organizations, creates GCP projects via Crossplane, resolves user details for members and approvers, and creates IAM permissions using the `IAMUserPermissions` CRD. It also responds to updates in `organization.status.members/approvers` to ensure IAM permissions are up-to-date.
   - **Integration**: Ensures GCP projects are created and IAM permissions are managed effectively.

3. **Assets Controller**
   - **Responsibilities**: Manages the lifecycle of assets, including provisioning resource claims stored in Bitbucket via Argo CD.
   - **Integration**: Uses Argo CD to sync deployment manifests from Bitbucket, ensuring the desired state of assets is maintained in the Kubernetes cluster.

4. **IAMUserPermissions Controller**
   - **Responsibilities**: Manages IAM roles and policy bindings for users, ensuring permissions are correctly set.
   - **Integration**: Manages user permissions within GCP projects based on product and asset relationships.

5. **ElevatedAccessRequest Controller**
   - **Responsibilities**: Manages the lifecycle of elevated access requests, including granting and revoking access based on TTL expiration.
   - **Integration**: Handles temporary elevated permissions for users, ensuring requests are approved and tracked properly.

#### Relationships

- **Between Custom Components**:
  - Organizations reference parent organizations and are referenced by products.
  - Products reference organizations and manage multiple assets.
  - Assets reference products and specify deployment claims in Bitbucket.
  - IAMUserPermissions associate users with products and assets, managing their IAM roles and policies.
  - ElevatedAccessRequests manage temporary elevated access for users, associated with products and assets.

- **With Crossplane**:
  - Products controller creates and manages GCP projects via Crossplane.
  - Crossplane is used to provision and manage GCP resources, integrated with Kubernetes control plane.

- **With GCP**:
  - IAMUserPermissions and ElevatedAccessRequests controllers manage IAM roles and policies for users in GCP projects.
  - GCP projects are created and managed for each product, ensuring proper resource isolation and access control.

- **With Bitbucket**:
  - Assets reference deployment claims stored in Bitbucket, enabling version-controlled resource definitions.
  - Argo CD is used to sync the deployment manifests from Bitbucket to the Kubernetes cluster, ensuring the desired state is maintained.

### Assets

#### CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: assets.registry.cloud.company.com
spec:
  group: cloud.company.com
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            metadata:
              type: object
              properties:
                labels:
                  type: object
                annotations:
                  type: object
                finalizers:
                  type: array
                  items:
                    type: string
            spec:
              type: object
              properties:
                assetID:
                  type: string
                  description: A globally unique identifier that is generated for the Asset.
                  x-kubernetes-preserve-unknown-fields: true
                  readOnly: true
                name:
                  type: string
                  description: Human-readable name of this Asset. Must be unique within a Product’s set of assets.
                  minLength: 1
                product:
                  type: object
                  properties:
                    apiVersion:
                      type: string
                      enum:
                        - cloud.company.com/v1alpha1
                    kind:
                      type: string
                      enum:
                        - Product
                    name:
                      type: string
                      minLength: 1
                  required:
                    - apiVersion
                    - kind
                    - name
                  description: Reference to the Product that this Asset belongs to.
                claims:
                  type: object
                  properties:
                    repositoryURL:
                      type: string
                      description: The URL of the Bitbucket repository containing Crossplane claims.
                    gitBranch:
                      type: string
                      description: The branch in the repository representing the claims.
                    gitCommit:
                      type: string
                      description: The commit identifier that produced the claims.
                  required:
                    - repositoryURL
                    - gitBranch
                    - gitCommit
                  description: Defines the resource(s) to be deployed from a Bitbucket repository containing Crossplane claims.
              required:
                - name
                - claims
                - product
            status:
              type: object
              properties:
                state:
                  type: string
                  description: The current state of the Asset.
                  enum: ["Pending", "Creating", "Ready", "Error"]
                message:
                  type: string
                  description: Any message related to the current state of the Asset.
                lastUpdated:
                  type: string
                  format: date-time
                  description: The last time the status was updated.
  scope: Namespaced
  names:
    plural: assets
    singular: asset
    kind: Asset
```

#### Controller

```go
func (r *AssetReconciler) reconcileArgoCDApplication(ctx context.Context, asset *assetsv1alpha1.Asset) error {
  // Create the Argo CD Application object
  app := &argov1alpha1.Application{
    ObjectMeta: metav1.ObjectMeta{
      Name:      fmt.Sprintf("%s-claims", asset.Name),
      Namespace: "argocd",
    },
    Spec: argov1alpha1.ApplicationSpec{
      Project: "default",
      Source: argov1alpha1.ApplicationSource{
        RepoURL:        asset.Spec.Claims.RepositoryURL,
        TargetRevision: asset.Spec.Claims.GitCommit,
        Path:           "claims",
      },
      Destination: argov1alpha1.ApplicationDestination{
        Server:    "https://kubernetes.default.svc",
        Namespace: asset.Namespace,
      },
      SyncPolicy: &argov1alpha1.SyncPolicy{
        Automated: &argov1alpha1.SyncPolicyAutomated{
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
```

### Products

#### CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: products.registry.cloud.company.com
spec:
  group: cloud.company.com
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            metadata:
              type: object
              properties:
                labels:
                  type: object
                annotations:
                  type: object
                finalizers:
                  type: array
                  items:
                    type: string
            spec:
              type: object
              properties:
                name:
                  type: string
                  description: Human-readable name of this Product. Must be globally unique.
                  minLength: 1
                description:
                  type: string
                  description: Reader-friendly summary of this Product’s intended purpose.
                  minLength: 1
                organization:
                  type: object
                  properties:
                    apiVersion:
                      type: string
                      enum:
                        - cloud.company.com/v1alpha1
                    kind:
                      type: string
                      enum:
                        - Organization
                    name:
                      type: string
                      minLength: 1
                  required:
                    - apiVersion
                    - kind
                    - name
                  description: Reference to the Organization that this Product belongs to.
              required:
                - name
                - description
                - organization
            status:
              type: object
              properties:
                state:
                  type: string
                  description: The current state of the Product.
                  enum: ["Pending", "Creating", "Ready", "Error"]
                message:
                  type: string
                  description: Any message related to the current state of the Product.
                lastUpdated:
                  type: string
                  format: date-time
                  description: The last time the status was updated.
  scope: Namespaced
  names:
    plural: products
    singular: product
    kind: Product
```

#### Controller

Key  Responsibilities:
1. Validate that its organization exists.
2. Create/verify non-prod and prod GCP projects via Crossplane.
3. Resolve user details for all members in product.organization.members.
4. For all resolved members and approvers, create IAM permissions for default read-only access in the non-prod project.

```go

package controllers

import (
  "context"
  "fmt"
  "hash/fnv"
  "time"

  "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
  crossplanev1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
  "github.com/crossplane/provider-gcp/apis/v1beta1"
  prodv1alpha1 "path/to/products/api/v1alpha1"
  orgv1alpha1 "path/to/organizations/api/v1alpha1"
  iampv1alpha1 "path/to/iamuserpermissions/api/v1alpha1"
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
      Name: nonProdProjectID,
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
      Name: prodProjectID,
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
```

### Organizations

#### CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: organizations.registry.cloud.company.com
spec:
  group: cloud.company.com
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            metadata:
              type: object
              properties:
                labels:
                  type: object
                annotations:
                  type: object
                finalizers:
                  type: array
                  items:
                    type: string
            spec:
              type: object
              properties:
                name:
                  type: string
                  description: A human-readable name. Must be unique within a parent Organization.
                  minLength: 1
                parentOrg:
                  type: object
                  properties:
                    apiVersion:
                      type: string
                      enum:
                        - cloud.company.com/v1alpha1
                    kind:
                      type: string
                      enum:
                        - Organization
                    name:
                      type: string
                      minLength: 1
                  required:
                    - apiVersion
                    - kind
                    - name
                  description: Parent organization where the cost center for this organization is specified.
                memberOf:
                  type: array
                  items:
                    type: object
                    properties:
                      apiVersion:
                        type: string
                        enum:
                          - cloud.company.com/v1alpha1
                      kind:
                        type: string
                        enum:
                          - Organization
                      name:
                        type: string
                        minLength: 1
                  required:
                    - apiVersion
                    - kind
                    - name
                  description: References to organizations that have indirect authority over this organization.
                  default: []
                approvers:
                  type: string
                  description: An ACG group (a group of AD users) or an AGG group (a group of ACGs) containing users who can approve a production change.
                  minLength: 1
                members:
                  type: string
                  description: An ACG or AGG group representing members of this organization.
                  minLength: 1
              required:
                - name
                - ownedBy
                - approvers
                - members
            status:
              type: object
              properties:
                state:
                  type: string
                  description: The current state of the Organization.
                  enum: ["Pending", "Creating", "Ready", "Error"]
                message:
                  type: string
                  description: Any message related to the current state of the Organization.
                lastUpdated:
                  type: string
                  format: date-time
                  description: The last time the status was updated.
                approvers:
                  type: array
                  items:
                    type: string
                  description: List of resolved AD user details for approvers.
                members:
                  type: array
                  items:
                    type: string
                  description: List of resolved AD user details for members.
  scope: Namespaced
  names:
    plural: organizations
    singular: organization
    kind: Organization
    shortNames:
      - org
```

#### Controller

```go

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
```

### IAM User Permissions

#### CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: iamuserpermissions.registry.cloud.company.com
spec:
  group: cloud.company.com
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            metadata:
              type: object
              properties:
                labels:
                  type: object
                annotations:
                  type: object
                finalizers:
                  type: array
                  items:
                    type: string
            spec:
              type: object
              properties:
                userEmail:
                  type: string
                  description: Email of the user for whom the IAM permissions are being managed.
                  minLength: 1
                productName:
                  type: string
                  description: Name of the product or project.
                  minLength: 1
                assetNames:
                  type: array
                  items:
                    type: string
                  description: List of assets associated with the product.
                permissions:
                  type: array
                  items:
                    type: string
                  description: List of IAM permissions for the user.
              required:
                - userEmail
                - productName
                - permissions
            status:
              type: object
              properties:
                state:
                  type: string
                  description: The current state of the IAMUserPermissions.
                  enum: ["Pending", "Active", "Error"]
                message:
                  type: string
                  description: Any message related to the current state of the IAMUserPermissions.
                lastUpdated:
                  type: string
                  format: date-time
                  description: The last time the status was updated.
  scope: Namespaced
  names:
    plural: iamuserpermissions
    singular: iamuserpermission
    kind: IAMUserPermission
    shortNames:
      - iamp
```

### Elevated Access Request

#### CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: elevatedaccessrequests.registry.cloud.company.com
spec:
  group: cloud.company.com
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            metadata:
              type: object
              properties:
                labels:
                  type: object
                annotations:
                  type: object
                finalizers:
                  type: array
                  items:
                    type: string
            spec:
              type: object
              properties:
                userEmail:
                  type: string
                  description: Email of the user requesting elevated access.
                  minLength: 1
                productName:
                  type: string
                  description: Name of the product or project.
                  minLength: 1
                assetNames:
                  type: array
                  items:
                    type: string
                  description: List of assets associated with the product.
                ttl:
                  type: string
                  description: Time-to-live for the elevated access request.
                approvedBy:
                  type: string
                  description: Email of the user who approved the request.
                  minLength: 1
                elevatedPermissions:
                  type: array
                  items:
                    type: string
                  description: List of elevated IAM permissions for the user.
              required:
                - userEmail
                - productName
                - ttl
                - approvedBy
                - elevatedPermissions
            status:
              type: object
              properties:
                state:
                  type: string
                  description: The current state of the ElevatedAccessRequest.
                  enum: ["Pending", "Approved", "Active", "Expired", "Error"]
                message:
                  type: string
                  description: Any message related to the current state of the ElevatedAccessRequest.
                lastUpdated:
                  type: string
                  format: date-time
                  description: The last time the status was updated.
  scope: Namespaced
  names:
    plural: elevatedaccessrequests
    singular: elevatedaccessrequest
    kind: ElevatedAccessRequest
    shortNames:
      - ear
```

### Controller

Key Responsibilities:

	1.	Handle creation and update of elevated access requests.
	2.	Manage the lifecycle of elevated access, including TTL expiration.

```go
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
```
