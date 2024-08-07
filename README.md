# A2Y-D5L / enterprisectl

`enterprisectl` is a proof-of-concept design for an enterprise cloud control plane. It leverages Kubernetes custom resources and controllers to manage organizational structures, products, assets, and IAM permissions. It integrates with Crossplane for managing GCP resources and Argo CD for GitOps-based synchronization of asset claims stored in Bitbucket.

## Problem

Managing complex cloud environments in an enterprise setting involves handling multiple organizational units, products, assets, and access control policies. Ensuring consistency, security, and efficient resource management across these entities can be challenging, especially with the need for integration with various cloud services and infrastructure tools.

## Solution

`enterprisectl` provides a comprehensive solution by using Kubernetes custom resources and controllers to represent and manage these entities. By integrating with Crossplane and Argo CD, it ensures a declarative, automated, and secure approach to managing cloud resources and organizational hierarchies.

## System Components

### `enterprisectl` Components

1. **Organizations**

   Represent organizational units within the company, detailing ownership, hierarchical structure, and access control groups.

2. **Products**
  
   Represent products within an organization, each associated with GCP projects and encompassing multiple assets.

3. **Assets**

   Represent resources that comprise a product, detailing resource deployment information and their state.

4. **User Permissions**

   Maps IAM permissions between users, products, and assets, managing roles and policy bindings.

5. **Elevated Access Requests**

   Manages requests for temporary elevated access, including their lifecycle and expiration.

### Dependencies

1. **Crossplane**

   Used to provision and manage GCP resources within the Kubernetes control plane, ensuring proper resource isolation and access control.
2. **Argo CD**

   • Enables declarative management of Kubernetes resources by syncing the desired state from a Git repository.

   • Automates the synchronization of resources and ensures any drift from the desired state is corrected.

### Component Relationships

- Organizations are hierarchical and can reference each other.
- Products are associated with organizations and contain multiple assets.
- Assets are linked to products and are managed declaratively through GitOps.
- IAM User Permissions link users to products and assets, managing their access and roles.
- Elevated Access Requests handle temporary elevated permissions for users, ensuring proper approval and lifecycle management.
